package config

// TmuxConfig describes tmux windows created for a worktree.
type TmuxConfig struct {
	Windows []TmuxWindow
}

// TmuxWindow describes a named tmux window and its panes.
type TmuxWindow struct {
	Name  string
	Panes []TmuxPane
}

// TmuxPane describes a tmux pane. Command is run in the pane after it is
// created. An empty command leaves the pane running its default shell.
type TmuxPane struct {
	Command string
}

type rawConfig struct {
	Tmux *rawTmuxConfig `yaml:"tmux"`
}

type rawTmuxConfig struct {
	Windows []rawTmuxWindow `yaml:"windows"`
}

type rawTmuxWindow struct {
	Name  string        `yaml:"name"`
	Panes []rawTmuxPane `yaml:"panes"`
}

type rawTmuxPane struct {
	Command string `yaml:"command"`
}
