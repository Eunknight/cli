// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"context"
	"fmt"
	"net/http"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// MinutesUpload uploads a media file token to generate a minute.
var MinutesUpload = common.Shortcut{
	Service:     "minutes",
	Command:     "+upload",
	Description: "Upload a media file token to generate a minute",
	Risk:        "write",
	Scopes:      []string{"minutes:minutes.upload:write"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "file-token", Desc: "already uploaded media file_token from drive", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := runtime.Str("file-token")
		if fileToken == "" {
			return output.ErrValidation("--file-token is required")
		}
		if err := validate.ResourceName(fileToken, "--file-token"); err != nil {
			return output.ErrValidation("%s", err)
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			POST("/open-apis/minutes/v1/minutes/upload").
			Body(map[string]interface{}{"file_token": runtime.Str("file-token")})
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		fileToken := runtime.Str("file-token")

		body := map[string]interface{}{
			"file_token": fileToken,
		}

		data, err := runtime.CallAPI(http.MethodPost, "/open-apis/minutes/v1/minutes/upload", nil, body)
		if err != nil {
			return err
		}

		minuteURL := common.GetString(data, "minute_url")

		fmt.Fprintln(runtime.IO().ErrOut, "Minute generation started successfully.")
		fmt.Fprintln(runtime.IO().ErrOut, "Note: The minute content is generated asynchronously. The link may not be immediately ready for access. You will receive a notification in Lark when it is fully transcribed.")

		outData := map[string]interface{}{
			"minute_url": minuteURL,
		}

		runtime.OutFormat(outData, nil, nil)
		return nil
	},
}
