package app

import (
	"errors"
	"fmt"
	"github.com/angelmsger/openobserve-cli/internal/config"
	"os"
	"strings"

	"github.com/angelmsger/openobserve-cli/internal/auth"
	"github.com/angelmsger/openobserve-cli/pkg/constants"
	"github.com/charmbracelet/huh"
)

// initValues are the inputs collected by the config-init flow, whether through
// the interactive TUI (--pretty) or plain line prompts.
type initValues struct {
	credentialURL string
	baseURL       string
	org           string
	scheme        string
	email         string // basic scheme only
	secret        string // password (basic) or token value (token)
}

// withDefaults seeds empty fields with sensible defaults so the wizard shows
// them pre-filled.
func (v initValues) withDefaults() initValues {
	if v.baseURL == "" {
		v.baseURL = constants.SelfHostedBaseURL
	}
	if v.org == "" {
		v.org = constants.DefaultOrg
	}
	if v.scheme == "" {
		v.scheme = auth.SchemeBasic
	}
	return v
}

// runInitForm presents the interactive huh TUI to collect config-init values,
// pre-seeded with def. The form renders to stderr (and reads stdin), so the
// command's JSON result on stdout stays a clean data channel.
func runInitForm(def initValues) (initValues, error) {
	v := def.withDefaults()
	serviceForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Server URL").
				Placeholder(constants.SelfHostedBaseURL).
				Value(&v.baseURL).
				Validate(required),
			huh.NewInput().
				Title("Organization").
				Value(&v.org),
			huh.NewSelect[string]().
				Title("Auth scheme").
				Options(
					huh.NewOption("basic — email + password", auth.SchemeBasic),
					huh.NewOption("token — pre-generated (SSO / service account)", auth.SchemeToken),
				).
				Value(&v.scheme),
		),
	).WithInput(os.Stdin).WithOutput(os.Stderr)
	if err := serviceForm.Run(); err != nil {
		return v, err
	}
	guidance, err := wizardGuide(v)
	if err != nil {
		return v, err
	}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Email").
				Value(&v.email).
				Validate(required),
			huh.NewInput().
				Title("Password").
				Description(strings.Join(guidance.Lines(), "\n")).
				EchoMode(huh.EchoModePassword).
				Value(&v.secret).
				Validate(required),
		).WithHideFunc(func() bool { return v.scheme != auth.SchemeBasic }),
		huh.NewGroup(
			huh.NewInput().
				Title("Token").
				Description(strings.Join(guidance.Lines(), "\n")).
				EchoMode(huh.EchoModePassword).
				Value(&v.secret).
				Validate(required),
		).WithHideFunc(func() bool { return v.scheme != auth.SchemeToken }),
	).WithInput(os.Stdin).WithOutput(os.Stderr)

	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return v, fmt.Errorf("setup cancelled")
		}
		return v, err
	}
	return v, nil
}

// runInitPrompts collects the same values through plain line prompts (stderr),
// used when --pretty is not set.
func runInitPrompts(def initValues) (initValues, error) {
	v := def.withDefaults()
	var err error
	if v.baseURL, err = promptLine("Server URL", v.baseURL); err != nil {
		return v, err
	}
	if v.org, err = promptLine("Organization", v.org); err != nil {
		return v, err
	}
	if v.scheme, err = promptLine("Auth scheme (basic/token)", v.scheme); err != nil {
		return v, err
	}
	if g, e := wizardGuide(v); e != nil {
		return v, e
	} else {
		for _, line := range g.Lines() {
			fmt.Fprintln(os.Stderr, line)
		}
	}
	switch v.scheme {
	case auth.SchemeBasic:
		if v.email, err = promptLine("Email", v.email); err != nil {
			return v, err
		}
		if v.secret, err = promptSecret("Password"); err != nil {
			return v, err
		}
	case auth.SchemeToken:
		if v.secret, err = promptSecret("Token"); err != nil {
			return v, err
		}
	}
	return v, nil
}

func required(s string) error {
	if s == "" {
		return errors.New("required")
	}
	return nil
}

// formSelect runs a single-select huh form (rendered to stderr, reading stdin),
// used by `config init` to ask the edit/add/replace action or which context to
// edit. It returns the selected option value.
func formSelect(title string, options []huh.Option[string], def string) (string, error) {
	val := def
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().Title(title).Options(options...).Value(&val),
		),
	).WithInput(os.Stdin).WithOutput(os.Stderr)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", fmt.Errorf("setup cancelled")
		}
		return "", err
	}
	return val, nil
}

// formInput runs a single free-text huh input (e.g. a new context name).
func formInput(title, placeholder string) (string, error) {
	var val string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().Title(title).Placeholder(placeholder).Value(&val),
		),
	).WithInput(os.Stdin).WithOutput(os.Stderr)
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return "", fmt.Errorf("setup cancelled")
		}
		return "", err
	}
	return val, nil
}

func wizardGuide(v initValues) (config.AuthGuide, error) {
	return config.Guide(config.Config{BaseURL: v.baseURL, Auth: config.AuthConfig{Scheme: v.scheme, CredentialURL: v.credentialURL}}, nil)
}
