package app

import (
	"context"
	"encoding/json"
	"io"
	"os"

	"github.com/angelmsger/openobserve-cli/pkg/apiclient"
	cerrors "github.com/angelmsger/openobserve-cli/pkg/errors"
)

// readDefinitionBody loads --file (or stdin with -) for metadata writes.
func readDefinitionBody(file, noun string) (json.RawMessage, error) {
	if file == "" {
		return nil, cerrors.New(cerrors.CategoryUsage, "BODY_FILE_REQUIRED",
			"a "+noun+" definition file is required").
			WithNextSteps("--file "+noun+".json", "--file -  (read from stdin)")
	}
	var data []byte
	var err error
	if file == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		name := file
		if len(name) > 1 && name[0] == '@' {
			name = name[1:]
		}
		data, err = os.ReadFile(name)
	}
	if err != nil {
		return nil, cerrors.Wrap(err, cerrors.CategoryUsage, "FILE_READ",
			"could not read "+file)
	}
	if len(data) == 0 {
		return nil, cerrors.New(cerrors.CategoryUsage, "EMPTY_BODY",
			"the "+noun+" definition file is empty")
	}
	return json.RawMessage(data), nil
}

// emitWritePlan delegates preview construction to the same API builders used
// for execution. This client has no transport or credentials and cannot send HTTP.
func (s *appState) emitWritePlan(op any) error {
	base, err := apiclient.NormalizeBaseURL(s.cfg().BaseURL)
	if err != nil {
		return err
	}
	client := apiclient.New(apiclient.Config{BaseURL: base})
	plan, err := client.DescribeWrite(context.Background(), op)
	if err != nil {
		return err
	}
	return s.emit(map[string]any{"dry_run": true, "request": plan})
}

func folderOrDefault(folder string) string {
	if folder == "" {
		return "default"
	}
	return folder
}
