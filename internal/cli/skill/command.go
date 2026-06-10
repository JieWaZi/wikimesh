package skillcmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/JieWaZi/wikimesh/internal/app/skillapp"
	"github.com/JieWaZi/wikimesh/internal/ui"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	skillSelectOne         = ui.SelectOne
	skillSearchMultiselect = ui.SearchMultiselect
)

// InstallOptions 描述 `wikimesh skill install` 的安装参数。
type InstallOptions struct {
	// Agent 是目标 Agent 的逗号分隔文本。
	Agent string
	// WikiType 是要安装的 skill 类型。
	WikiType string
	// Yes 表示跳过交互选择并安装全部可用 skills。
	Yes bool
	// AgentProvided 表示用户显式传入了 agent 参数。
	AgentProvided bool
	// WikiTypeProvided 表示用户显式传入了 skill 类型参数。
	WikiTypeProvided bool
}

// NewCommand 构造 `wikimesh skill` 命令。
func NewCommand() *cobra.Command {
	msg := ui.Messages()
	cmd := &cobra.Command{
		Use:   "skill",
		Short: msg.WikiSkillShort,
		Run: func(cmd *cobra.Command, args []string) {
			_ = cmd.Help()
		},
	}
	cmd.AddCommand(newInstallCommand())
	return cmd
}

func newInstallCommand() *cobra.Command {
	msg := ui.Messages()
	var opts InstallOptions
	cmd := &cobra.Command{
		Use:   "install [version]",
		Short: msg.WikiSkillInstallShort,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := ""
			if len(args) > 0 {
				ref = args[0]
			}
			opts.AgentProvided = cmd.Flags().Changed("agent")
			opts.WikiTypeProvided = cmd.Flags().Changed("type")
			interactive := !opts.Yes && readerIsTerminal(cmd.InOrStdin())
			return runWikiSkillInstall(cmd.OutOrStdout(), interactive, opts, ref)
		},
	}
	cmd.Flags().StringVar(&opts.Agent, "agent", "", msg.FlagAgent)
	cmd.Flags().StringVar(&opts.WikiType, "type", "", msg.FlagWikiType)
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, msg.FlagYes)
	return cmd
}

func readerIsTerminal(r any) bool {
	file, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

// runWikiSkillInstall 从 GitHub 安装最新 Wikimesh runtime skills。
func runWikiSkillInstall(out io.Writer, interactive bool, opts InstallOptions, ref string) error {
	if interactive && !opts.AgentProvided {
		agents, err := promptSkillInstallAgents()
		if err != nil {
			return err
		}
		opts.Agent = strings.Join(agents, ",")
	}
	agents, err := normalizeSkillInstallAgents(opts.Agent)
	if err != nil {
		return err
	}
	if !interactive && strings.TrimSpace(opts.WikiType) == "" {
		opts.WikiType = skillapp.DefaultType()
	}
	if !opts.WikiTypeProvided && interactive {
		opts.WikiType = ""
	}
	selected, cleanup, wikiType, err := selectedSkillInstallSkills(opts.WikiType, ref, interactive, opts.Yes)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer func() { _ = cleanup() }()
	}
	for _, agent := range agents {
		if err := skillapp.Install(agent, false, selected); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(out, ui.Messages().OutputWikiSkillsInstalledFmt, strings.Join(agents, ","), wikiType, len(selected))
	return err
}

// promptSkillInstallAgents 交互式收集 skill install 的目标 agent。
func promptSkillInstallAgents() ([]string, error) {
	msg := ui.Messages()
	values, cancelled, err := skillSearchMultiselect(ui.SearchMultiselectOptions{
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
		return nil, err
	}
	if cancelled {
		return nil, errors.New(msg.Cancelled)
	}
	return values, nil
}

// normalizeSkillInstallAgents 归一化 skill install 的 agent 参数。
func normalizeSkillInstallAgents(agentText string) ([]string, error) {
	msg := ui.Messages()
	values := splitCommaValues(agentText)
	if len(values) == 0 {
		values = []string{"codex"}
	}
	agents := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		agent := strings.TrimSpace(value)
		if agent == "" {
			continue
		}
		switch agent {
		case "codex", "cursor", "claude":
		default:
			return nil, fmt.Errorf(msg.ErrorUnsupportedAgentFmt, agent)
		}
		if _, ok := seen[agent]; ok {
			continue
		}
		seen[agent] = struct{}{}
		agents = append(agents, agent)
	}
	if len(agents) == 0 {
		agents = []string{"codex"}
	}
	return agents, nil
}

// selectedSkillInstallSkills 下载并按交互选择过滤 Wikimesh runtime skills。
func selectedSkillInstallSkills(wikiType string, ref string, interactive bool, yes bool) ([]skillapp.Skill, func() error, string, error) {
	if strings.TrimSpace(wikiType) == "" && interactive {
		items := make([]ui.Option, 0, len(skillapp.BuiltinTypes()))
		for _, typ := range skillapp.BuiltinTypes() {
			items = append(items, ui.Option{Value: typ.Name, Label: typ.Name, Hint: typ.Description})
		}
		value, cancelled, err := skillSelectOne(ui.SelectOneOptions{
			Message: ui.Messages().PromptWikiType,
			Items:   items,
		})
		if err != nil {
			return nil, nil, "", err
		}
		if cancelled {
			return nil, nil, "", errors.New(ui.Messages().Cancelled)
		}
		wikiType = value
	}
	if strings.TrimSpace(wikiType) == "" {
		wikiType = skillapp.DefaultType()
	}
	source := skillapp.NewSource(wikiType, ref)
	ui.Step(fmt.Sprintf(ui.Messages().StepFetchingWikiSkillsFmt, wikiSkillSourceDisplay(source)))
	bundle, err := skillapp.ResolveWikimeshSkills(wikiType, ref)
	if err != nil {
		return nil, nil, "", err
	}
	selected, err := resolveSelectedSkillInstallSkills(bundle.Skills, interactive, yes)
	if err != nil {
		if bundle.Cleanup != nil {
			_ = bundle.Cleanup()
		}
		return nil, nil, "", err
	}
	return selected, bundle.Cleanup, wikiType, nil
}

// wikiSkillSourceDisplay 将 skill 来源转成用户可直接访问的展示地址。
func wikiSkillSourceDisplay(source skillapp.Source) string {
	repo := strings.TrimSuffix(source.RepoURL, ".git")
	if source.Ref == "" || source.Subpath == "" {
		return source.Original
	}
	return fmt.Sprintf("%s/tree/%s/%s", repo, source.Ref, source.Subpath)
}

// resolveSelectedSkillInstallSkills 按交互状态决定要安装哪些 Wikimesh skills。
func resolveSelectedSkillInstallSkills(found []skillapp.Skill, interactive bool, yes bool) ([]skillapp.Skill, error) {
	if len(found) == 0 {
		return nil, nil
	}
	if !interactive || yes {
		return append([]skillapp.Skill(nil), found...), nil
	}
	items := make([]ui.Option, 0, len(found))
	initial := make([]string, 0, len(found))
	for _, skill := range found {
		items = append(items, ui.Option{Value: skill.Name, Label: skill.Name, Hint: skill.Description})
		initial = append(initial, skill.Name)
	}
	selectedNames, cancelled, err := skillSearchMultiselect(ui.SearchMultiselectOptions{
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
