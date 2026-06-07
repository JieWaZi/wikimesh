package qmd

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AddCollection 登记或更新一个 collection。
// 该方法只保存扫描规则和上下文配置；真正的文档读取、解析、索引在 UpdateCollection 中完成。
func (s *Store) AddCollection(ctx context.Context, c Collection) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("collection name is required")
	}
	if strings.TrimSpace(c.Path) == "" {
		return errors.New("collection path is required")
	}
	absPath, err := filepath.Abs(c.Path)
	if err != nil {
		return err
	}
	pattern := strings.TrimSpace(c.Pattern)
	if pattern == "" && len(c.Include) == 0 {
		pattern = defaultCollectionPattern
	}
	include := c.Include
	if pattern != "" && len(include) == 0 {
		include = []string{pattern}
	}
	contextJSON, err := encodeContext(c.Context)
	if err != nil {
		return err
	}
	includeByDefault := 1
	if c.IncludeByDefault != nil && !*c.IncludeByDefault {
		includeByDefault = 0
	}
	now := time.Now().UTC().Format(time.RFC3339)
	return s.db.WriteTx(func(tx *sql.Tx) error {
		_, err := tx.Exec(`
INSERT INTO collections
	(name, path, pattern, include_globs, ignore_globs, update_command, include_by_default, context_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(name) DO UPDATE SET
	path=excluded.path,
	pattern=excluded.pattern,
	include_globs=excluded.include_globs,
	ignore_globs=excluded.ignore_globs,
	update_command=excluded.update_command,
	include_by_default=excluded.include_by_default,
	context_json=CASE
		WHEN excluded.context_json='{}' THEN collections.context_json
		ELSE excluded.context_json
	END,
	updated_at=excluded.updated_at
`, c.Name, absPath, pattern, strings.Join(include, "\n"), strings.Join(c.Ignore, "\n"), c.Update, includeByDefault, contextJSON, now, now)
		return err
	})
}

// ListCollections 返回所有 collection 配置。
func (s *Store) ListCollections(ctx context.Context) ([]Collection, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.db.ReadDB().Query(`
SELECT
	c.name,
	c.path,
	c.pattern,
	c.include_globs,
	c.ignore_globs,
	c.update_command,
	c.include_by_default,
	c.context_json,
	COUNT(d.id) AS doc_count,
	COALESCE(SUM(CASE WHEN d.active != 0 THEN 1 ELSE 0 END), 0) AS active_count,
	COALESCE(MAX(d.updated_at), '') AS last_modified
FROM collections c
LEFT JOIN collection_documents d ON d.collection = c.name
GROUP BY c.name
ORDER BY c.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var collections []Collection
	for rows.Next() {
		var c Collection
		var include, ignore, contextJSON string
		var includeByDefault int
		if err := rows.Scan(&c.Name, &c.Path, &c.Pattern, &include, &ignore, &c.Update, &includeByDefault, &contextJSON, &c.DocCount, &c.ActiveCount, &c.LastModified); err != nil {
			return nil, err
		}
		c.Include = splitLines(include)
		c.Ignore = splitLines(ignore)
		c.IncludeByDefault = BoolPtr(includeByDefault != 0)
		c.Context = decodeContext(contextJSON)
		collections = append(collections, c)
	}
	return collections, rows.Err()
}

// RemoveCollection 删除 collection 元数据和它拥有的索引内容。
func (s *Store) RemoveCollection(ctx context.Context, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	existing, err := s.getCollection(name)
	if err != nil {
		if strings.HasPrefix(err.Error(), "collection not found:") {
			return false, nil
		}
		return false, err
	}
	return true, s.db.WriteTx(func(tx *sql.Tx) error {
		return s.deleteCollectionDocumentsTx(tx, existing.Name)
	})
}

// RenameCollection 重命名 collection，并同步已索引文档的稳定 ID 和全文索引路径。
func (s *Store) RenameCollection(ctx context.Context, oldName, newName string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if strings.TrimSpace(newName) == "" {
		return false, errors.New("new collection name is required")
	}
	oldName = strings.TrimSpace(oldName)
	newName = strings.TrimSpace(newName)
	returned := false
	err := s.db.WriteTx(func(tx *sql.Tx) error {
		var exists string
		err := tx.QueryRow(`SELECT name FROM collections WHERE name=?`, oldName).Scan(&exists)
		if err == sql.ErrNoRows {
			returned = false
			return nil
		}
		if err != nil {
			return err
		}
		err = tx.QueryRow(`SELECT name FROM collections WHERE name=?`, newName).Scan(&exists)
		if err == nil {
			return fmt.Errorf("collection already exists: %s", newName)
		}
		if err != sql.ErrNoRows {
			return err
		}

		rows, err := tx.Query(`SELECT id, rel_path FROM collection_documents WHERE collection=?`, oldName)
		if err != nil {
			return err
		}
		type renameDoc struct {
			oldID   string
			newID   string
			relPath string
		}
		var docs []renameDoc
		for rows.Next() {
			var doc renameDoc
			if err := rows.Scan(&doc.oldID, &doc.relPath); err != nil {
				rows.Close()
				return err
			}
			doc.newID = documentID(newName, doc.relPath)
			docs = append(docs, doc)
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()

		now := time.Now().UTC().Format(time.RFC3339)
		if _, err := tx.Exec(`
INSERT INTO collections
	(name, path, pattern, include_globs, ignore_globs, update_command, include_by_default, context_json, created_at, updated_at)
SELECT ?, path, pattern, include_globs, ignore_globs, update_command, include_by_default, context_json, created_at, ?
FROM collections
WHERE name=?
`, newName, now, oldName); err != nil {
			return err
		}
		for _, doc := range docs {
			if _, err := tx.Exec(`UPDATE collection_documents SET id=?, collection=?, updated_at=? WHERE id=?`, doc.newID, newName, now, doc.oldID); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE entries SET id=?, tags=? WHERE id=?`, doc.newID, newName, doc.oldID); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE chunks_meta SET doc_id=?, chunk_id=replace(chunk_id, ?, ?) WHERE doc_id=?`, doc.newID, doc.oldID+":", doc.newID+":", doc.oldID); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE chunks_fts SET chunk_id=replace(chunk_id, ?, ?) WHERE chunk_id LIKE ?`, doc.oldID+":", doc.newID+":", doc.oldID+":%"); err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE vec_chunks SET doc_id=?, chunk_id=replace(chunk_id, ?, ?) WHERE doc_id=?`, doc.newID, doc.oldID+":", doc.newID+":", doc.oldID); err != nil {
				return err
			}
			rowID, err := collectionDocumentRowID(tx, newName, doc.relPath)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`UPDATE documents_fts SET filepath=? WHERE rowid=?`, searchFilePath(newName, doc.relPath), rowID); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`DELETE FROM collections WHERE name=?`, oldName); err != nil {
			return err
		}
		returned = true
		return nil
	})
	return returned, err
}

// UpdateCollectionSettings 更新 qmd collection 的 update-cmd/includeByDefault 设置。
func (s *Store) UpdateCollectionSettings(ctx context.Context, name string, settings CollectionSettings) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	changed := false
	err := s.db.WriteTx(func(tx *sql.Tx) error {
		var exists string
		if err := tx.QueryRow(`SELECT name FROM collections WHERE name=?`, name).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return err
		}
		if settings.Update != nil {
			if _, err := tx.Exec(`UPDATE collections SET update_command=?, updated_at=? WHERE name=?`, *settings.Update, time.Now().UTC().Format(time.RFC3339), name); err != nil {
				return err
			}
		}
		if settings.IncludeByDefault != nil {
			value := 0
			if *settings.IncludeByDefault {
				value = 1
			}
			if _, err := tx.Exec(`UPDATE collections SET include_by_default=?, updated_at=? WHERE name=?`, value, time.Now().UTC().Format(time.RFC3339), name); err != nil {
				return err
			}
		}
		changed = true
		return nil
	})
	return changed, err
}

// DefaultCollectionNames 返回 qmd 默认查询会包含的 collection 名称。
func (s *Store) DefaultCollectionNames(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rows, err := s.db.ReadDB().Query(`SELECT name FROM collections WHERE include_by_default != 0 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

// SetGlobalContext 保存 qmd global_context。
func (s *Store) SetGlobalContext(ctx context.Context, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.db.WriteTx(func(tx *sql.Tx) error {
		if strings.TrimSpace(value) == "" {
			_, err := tx.Exec(`DELETE FROM collection_config WHERE key='global_context'`)
			return err
		}
		_, err := tx.Exec(`INSERT INTO collection_config (key, value) VALUES ('global_context', ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, value)
		return err
	})
}

// GlobalContext 返回 qmd global_context。
func (s *Store) GlobalContext(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var value string
	err := s.db.ReadDB().QueryRow(`SELECT value FROM collection_config WHERE key='global_context'`).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// AddContext 添加或更新 collection path context。
func (s *Store) AddContext(ctx context.Context, collectionName, pathPrefix, text string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	changed := false
	err := s.db.WriteTx(func(tx *sql.Tx) error {
		var raw string
		if err := tx.QueryRow(`SELECT context_json FROM collections WHERE name=?`, collectionName).Scan(&raw); err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return err
		}
		contexts := decodeContext(raw)
		if contexts == nil {
			contexts = map[string]string{}
		}
		contexts[pathPrefix] = text
		encoded, err := encodeContext(contexts)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE collections SET context_json=?, updated_at=? WHERE name=?`, encoded, time.Now().UTC().Format(time.RFC3339), collectionName)
		changed = err == nil
		return err
	})
	return changed, err
}

// RemoveContext 删除 collection path context。
func (s *Store) RemoveContext(ctx context.Context, collectionName, pathPrefix string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	changed := false
	err := s.db.WriteTx(func(tx *sql.Tx) error {
		var raw string
		if err := tx.QueryRow(`SELECT context_json FROM collections WHERE name=?`, collectionName).Scan(&raw); err != nil {
			if err == sql.ErrNoRows {
				return nil
			}
			return err
		}
		contexts := decodeContext(raw)
		if _, ok := contexts[pathPrefix]; !ok {
			return nil
		}
		delete(contexts, pathPrefix)
		encoded, err := encodeContext(contexts)
		if err != nil {
			return err
		}
		_, err = tx.Exec(`UPDATE collections SET context_json=?, updated_at=? WHERE name=?`, encoded, time.Now().UTC().Format(time.RFC3339), collectionName)
		changed = err == nil
		return err
	})
	return changed, err
}

// ListContexts 返回 global 和 collection path contexts。
func (s *Store) ListContexts(ctx context.Context) ([]CollectionContext, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var out []CollectionContext
	global, err := s.GlobalContext(ctx)
	if err != nil {
		return nil, err
	}
	if global != "" {
		out = append(out, CollectionContext{Collection: "*", Path: "/", Context: global})
	}
	rows, err := s.db.ReadDB().Query(`SELECT name, context_json FROM collections WHERE context_json != '{}' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var name, raw string
		if err := rows.Scan(&name, &raw); err != nil {
			return nil, err
		}
		contexts := decodeContext(raw)
		paths := make([]string, 0, len(contexts))
		for path := range contexts {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		for _, path := range paths {
			out = append(out, CollectionContext{Collection: name, Path: path, Context: contexts[path]})
		}
	}
	return out, rows.Err()
}

// ContextForPath 按 qmd 规则返回最具体 path context，找不到时回退 global_context。
func (s *Store) ContextForPath(ctx context.Context, collectionName, relPath string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	c, err := s.getCollection(collectionName)
	if err != nil {
		return "", err
	}
	normalizedPath := "/" + strings.TrimPrefix(filepath.ToSlash(relPath), "/")
	bestPrefix := ""
	bestContext := ""
	for prefix, value := range c.Context {
		normalizedPrefix := "/" + strings.TrimPrefix(filepath.ToSlash(prefix), "/")
		if strings.HasPrefix(normalizedPath, normalizedPrefix) && len(normalizedPrefix) > len(bestPrefix) {
			bestPrefix = normalizedPrefix
			bestContext = value
		}
	}
	if bestContext != "" {
		return bestContext, nil
	}
	return s.GlobalContext(ctx)
}

func (s *Store) getCollection(name string) (Collection, error) {
	var c Collection
	var include, ignore, contextJSON string
	var includeByDefault int
	err := s.db.ReadDB().QueryRow(
		`SELECT name, path, pattern, include_globs, ignore_globs, update_command, include_by_default, context_json FROM collections WHERE name=?`,
		name,
	).Scan(&c.Name, &c.Path, &c.Pattern, &include, &ignore, &c.Update, &includeByDefault, &contextJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return c, fmt.Errorf("collection not found: %s", name)
		}
		return c, err
	}
	c.Include = splitLines(include)
	c.Ignore = splitLines(ignore)
	c.IncludeByDefault = BoolPtr(includeByDefault != 0)
	c.Context = decodeContext(contextJSON)
	return c, nil
}

func (s *Store) deleteCollectionDocumentsTx(tx *sql.Tx, collection string) error {
	rows, err := tx.Query(`SELECT id FROM collection_documents WHERE collection=?`, collection)
	if err != nil {
		return err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, id := range ids {
		if err := s.chunkStore.DeleteDocChunks(tx, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM entries WHERE id=?`, id); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM documents_fts WHERE rowid IN (SELECT rowid FROM collection_documents WHERE id=?)`, id); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM collection_documents WHERE collection=?`, collection); err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM collections WHERE name=?`, collection)
	return err
}
