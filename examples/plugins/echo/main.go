// Echo is an independently built sidecar; it imports only the public SDK.
package main

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/seiortech/mtools-sdk/pluginapi"
)

func main() {
	identity := pluginapi.InitializeResult{
		Protocol: pluginapi.Protocol, PluginID: "org.example.echo", Version: "1.0.0",
	}
	runner := pluginapi.RunnerFunc(func(ctx context.Context, p pluginapi.RunParams, emit func(pluginapi.Progress) error) (pluginapi.RunResult, error) {
		if p.Tool != "echo" || p.Action != "echo" {
			return pluginapi.RunResult{}, errors.New("unsupported action")
		}
		pluginapi.LoggerFromContext(ctx).Info("echo action executing", "component", "echo", "count", len(p.Inputs["text"]))
		if err := emit(pluginapi.Progress{JobID: p.JobID, Done: 1, Total: 1, Unit: "actions", Message: "Echo complete"}); err != nil {
			return pluginapi.RunResult{}, err
		}
		if err := ctx.Err(); err != nil {
			return pluginapi.RunResult{}, err
		}
		return pluginapi.RunResult{
			JobID: p.JobID, Outputs: []string{}, Summary: strings.Join(p.Inputs["text"], " "),
		}, nil
	})
	if err := pluginapi.Serve(context.Background(), os.Stdin, os.Stdout, identity, runner); err != nil {
		// Do not print request contents or raw errors containing private inputs.
		os.Exit(1)
	}
}
