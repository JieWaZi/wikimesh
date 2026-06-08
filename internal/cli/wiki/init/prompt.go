package initcmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/app/skillapp"
	"github.com/JieWaZi/wikimesh/internal/app/wikiapp"
	"github.com/JieWaZi/wikimesh/internal/ui"
)

// collectInitOptions 在缺少 init 参数时按收集交互回答。
func collectInitOptions(in io.Reader, out io.Writer, interactive bool, opts *InitOptions, agentProvided, codeDirsProvided bool) error {
	msg := ui.Messages()
	reader := bufio.NewReader(in)
	if strings.TrimSpace(opts.Mode) == "" {
		if interactive {
			value, cancelled, err := wikiSelectOne(ui.SelectOneOptions{
				Message: msg.PromptWikiInitMode,
				Items: []ui.Option{
					{Value: InitModeCreate, Label: msg.WikiInitModeCreateLabel},
					{Value: InitModeLink, Label: msg.WikiInitModeLinkLabel},
				},
			})
			if err != nil {
				return err
			}
			if cancelled {
				return errors.New(msg.Cancelled)
			}
			opts.Mode = value
		} else {
			value, err := promptWikiLine(reader, out, msg.PromptWikiInitMode, InitModeCreate)
			if err != nil {
				return err
			}
			opts.Mode = value
		}
	}
	if strings.TrimSpace(opts.ProjectName) == "" {
		value, err := promptWikiLine(reader, out, msg.PromptWikiProjectName, "")
		if err != nil {
			return err
		}
		opts.ProjectName = value
	}
	_ = opts.WikiTypeProvided
	if !agentProvided {
		if interactive {
			values, cancelled, err := wikiSearchMultiselect(ui.SearchMultiselectOptions{
				Message: msg.PromptWikiAgent,
				Items: []ui.Option{
					{Value: "codex", Label: "Codex"},
					{Value: "cursor", Label: "Cursor"},
					{Value: "claude", Label: "Claude Code"},
				},
				InitialSelected: []string{"codex"},
				Required:        true,
				MaxVisible:      3,
			})
			if err != nil {
				return err
			}
			if cancelled {
				return errors.New(msg.Cancelled)
			}
			opts.Agents = values
			opts.Agent = strings.Join(values, ",")
		} else {
			value, err := promptWikiLine(reader, out, msg.PromptWikiAgent, "codex")
			if err != nil {
				return err
			}
			opts.Agent = value
			opts.Agents = splitCommaValues(value)
		}
	}
	if strings.TrimSpace(opts.Mode) == InitModeLink {
		if err := collectWikiLinkOptions(reader, out, interactive, opts, codeDirsProvided); err != nil {
			return err
		}
		if !opts.ScopeProvided {
			return collectWikiInitScope(reader, out, interactive, opts)
		}
		return nil
	}
	if !codeDirsProvided && len(opts.CodeDirs) == 0 {
		if interactive {
			if err := collectWikiCodeReposInteractive(reader, out, opts); err != nil {
				return err
			}
		} else {
			value, err := promptWikiLine(reader, out, msg.PromptWikiCodeDirs, "")
			if err != nil {
				return err
			}
			opts.CodeDirs = splitCommaValues(value)
		}
	}
	if !opts.ScopeProvided {
		return collectWikiInitScope(reader, out, interactive, opts)
	}
	return nil
}

// collectWikiInitScope 收集 runtime skill 安装范围。
func collectWikiInitScope(in *bufio.Reader, out io.Writer, interactive bool, opts *InitOptions) error {
	msg := ui.Messages()
	if interactive {
		value, cancelled, err := wikiSelectOne(ui.SelectOneOptions{
			Message: msg.PromptWikiScope,
			Items: []ui.Option{
				{Value: "project", Label: msg.InstallInProject},
				{Value: "global", Label: msg.InstallInHome},
			},
		})
		if err != nil {
			return err
		}
		if cancelled {
			return errors.New(msg.Cancelled)
		}
		opts.Global = value == "global"
		return nil
	}
	value, err := promptWikiLine(in, out, msg.PromptWikiScope, "project")
	if err != nil {
		return err
	}
	opts.Global = strings.EqualFold(strings.TrimSpace(value), "global")
	return nil
}

// collectWikiLinkOptions 收集关联已有文档库所需来源和可选代码库。
func collectWikiLinkOptions(in *bufio.Reader, out io.Writer, interactive bool, opts *InitOptions, codeDirsProvided bool) error {
	msg := ui.Messages()
	if strings.TrimSpace(opts.SourceType) == "" {
		if interactive {
			value, cancelled, err := wikiSelectOne(ui.SelectOneOptions{
				Message: msg.PromptWikiRepoSource,
				Items: []ui.Option{
					{Value: wikiapp.SourceLocal, Label: msg.WikiRepoSourceLocalLabel},
					{Value: wikiapp.SourceRemote, Label: msg.WikiRepoSourceRemoteLabel},
				},
			})
			if err != nil {
				return err
			}
			if cancelled {
				return errors.New(msg.Cancelled)
			}
			opts.SourceType = value
		} else {
			value, err := promptWikiLine(in, out, msg.PromptWikiRepoSource, wikiapp.SourceLocal)
			if err != nil {
				return err
			}
			opts.SourceType = value
		}
	}
	switch strings.TrimSpace(opts.SourceType) {
	case wikiapp.SourceLocal:
		if strings.TrimSpace(opts.LocalPath) == "" {
			defaultDir, err := os.Getwd()
			if err != nil {
				defaultDir = "."
			}
			value, err := promptWikiLine(in, out, msg.PromptWikiRepoLocalPath, defaultDir)
			if err != nil {
				return err
			}
			opts.LocalPath = value
		}
	case wikiapp.SourceRemote:
		if strings.TrimSpace(opts.RemoteURL) == "" {
			value, err := promptWikiLine(in, out, msg.PromptWikiRepoRemoteURL, "")
			if err != nil {
				return err
			}
			opts.RemoteURL = value
		}
	}
	if codeDirsProvided || len(opts.CodeDirs) > 0 {
		return nil
	}
	if interactive {
		return collectWikiCodeReposInteractive(in, out, opts)
	}
	value, err := promptWikiLine(in, out, msg.PromptWikiCodeDirs, "")
	if err != nil {
		return err
	}
	opts.CodeDirs = splitCommaValues(value)
	return nil
}

// collectWikiCodeReposInteractive 按循环收集可选代码库。
func collectWikiCodeReposInteractive(in io.Reader, out io.Writer, opts *InitOptions) error {
	msg := ui.Messages()
	linkedAny := false
	for {
		prompt := msg.PromptWikiRepoLinkCode
		items := []ui.Option{
			{Value: "link", Label: msg.WikiRepoLinkCodeLabel},
			{Value: "continue", Label: msg.WikiRepoContinueLabel},
		}
		if linkedAny {
			prompt = msg.PromptWikiRepoLinkMore
			items = []ui.Option{
				{Value: "link", Label: msg.WikiRepoLinkAnotherLabel},
				{Value: "finish", Label: msg.WikiRepoFinishLabel},
			}
		}
		choice, cancelled, err := wikiSelectOne(ui.SelectOneOptions{Message: prompt, Items: items})
		if err != nil {
			return err
		}
		if cancelled || choice == "continue" || choice == "finish" {
			return nil
		}
		repoSlug, err := promptWikiLine(in, out, msg.PromptWikiRepoCodeName, "")
		if err != nil {
			return err
		}
		repoPath, err := promptWikiLine(in, out, msg.PromptWikiRepoCodePath, ".")
		if err != nil {
			return err
		}
		opts.CodeRepos = append(opts.CodeRepos, InitCodeRepo{Slug: repoSlug, Path: repoPath})
		linkedAny = true
	}
}

// selectedWikiInitSkills 解析并按交互选择过滤 Wikimesh runtime skills。
func selectedWikiInitSkills(opts *InitOptions, interactive bool) ([]skillapp.Skill, func() error, error) {
	if strings.TrimSpace(opts.WikiType) == "" && interactive {
		items := make([]ui.Option, 0, len(skillapp.BuiltinTypes()))
		for _, typ := range skillapp.BuiltinTypes() {
			items = append(items, ui.Option{Value: typ.Name, Label: typ.Name, Hint: typ.Description})
		}
		value, cancelled, err := wikiSelectOne(ui.SelectOneOptions{
			Message: ui.Messages().PromptWikiType,
			Items:   items,
		})
		if err != nil {
			return nil, nil, err
		}
		if cancelled {
			return nil, nil, errors.New(ui.Messages().Cancelled)
		}
		opts.WikiType = value
	}
	if strings.TrimSpace(opts.WikiType) == "" {
		opts.WikiType = skillapp.DefaultType()
	}
	source := skillapp.NewSource(opts.WikiType, "")
	ui.Step(fmt.Sprintf(ui.Messages().StepFetchingWikiSkillsFmt, wikiSkillSourceDisplay(source)))
	bundle, err := skillapp.ResolveWikimeshSkills(opts.WikiType, "")
	if err != nil {
		return nil, nil, err
	}
	selected, err := resolveSelectedWikiSkills(bundle.Skills, *opts, interactive)
	if err != nil {
		if bundle.Cleanup != nil {
			_ = bundle.Cleanup()
		}
		return nil, nil, err
	}
	return selected, bundle.Cleanup, nil
}

// wikiSkillSourceDisplay 将 skill 来源转成用户可直接访问的展示地址。
func wikiSkillSourceDisplay(source skillapp.Source) string {
	repo := strings.TrimSuffix(source.RepoURL, ".git")
	if source.Ref == "" || source.Subpath == "" {
		return source.Original
	}
	return fmt.Sprintf("%s/tree/%s/%s", repo, source.Ref, source.Subpath)
}

// resolveSelectedWikiSkills 按交互状态决定要安装哪些 Wikimesh skills。
func resolveSelectedWikiSkills(found []skillapp.Skill, opts InitOptions, interactive bool) ([]skillapp.Skill, error) {
	if len(found) == 0 {
		return nil, nil
	}
	if !interactive || opts.Yes {
		return append([]skillapp.Skill(nil), found...), nil
	}

	items := make([]ui.Option, 0, len(found))
	initial := make([]string, 0, len(found))
	for _, skill := range found {
		items = append(items, ui.Option{Value: skill.Name, Label: skill.Name, Hint: skill.Description})
		initial = append(initial, skill.Name)
	}
	selectedNames, cancelled, err := wikiSearchMultiselect(ui.SearchMultiselectOptions{
		Message:         ui.Messages().PromptSelectWikiSkills,
		Items:           items,
		MaxVisible:      8,
		InitialSelected: initial,
		Required:        false,
	})
	if err != nil {
		return nil, err
	}
	if cancelled {
		return nil, nil
	}
	selectedSet := make(map[string]struct{}, len(selectedNames))
	for _, name := range selectedNames {
		selectedSet[name] = struct{}{}
	}
	selected := make([]skillapp.Skill, 0, len(selectedNames))
	for _, skill := range found {
		if _, ok := selectedSet[skill.Name]; ok {
			selected = append(selected, skill)
		}
	}
	return selected, nil
}

// promptWikiLine 输出提示并读取一行输入；空输入使用默认值。
func promptWikiLine(in io.Reader, out io.Writer, prompt string, defaultValue string) (string, error) {
	label := prompt
	if defaultValue != "" {
		label = fmt.Sprintf("%s [%s]", prompt, defaultValue)
	}
	if _, err := fmt.Fprintf(out, "%s: ", label); err != nil {
		return "", err
	}
	value, err := readPromptLine(in)
	if err != nil {
		return "", err
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

// readPromptLine 从输入流读取一行，优先复用外层传入的缓冲 reader。
func readPromptLine(in io.Reader) (string, error) {
	if reader, ok := in.(*bufio.Reader); ok {
		return reader.ReadString('\n')
	}
	return bufio.NewReader(in).ReadString('\n')
}

// splitCommaValues 将逗号分隔输入拆成非空列表。
func splitCommaValues(input string) []string {
	parts := strings.Split(input, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}
