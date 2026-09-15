package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/maestroi/gamevision/internal/inference"
)

func runModels(args []string) error {
	cmd := "list"
	name := ""
	rest := args
	if len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
		cmd = rest[0]
		rest = rest[1:]
		if cmd != "list" && len(rest) > 0 && !strings.HasPrefix(rest[0], "-") {
			name = rest[0]
			rest = rest[1:]
		}
	}

	fs := flag.NewFlagSet("models", flag.ContinueOnError)
	baseURL := fs.String("base-url", envOr("GAMEVISION_BASE_URL", "http://localhost:8002/v1"), "OpenAI-compatible VLM base URL")
	apiKey := fs.String("api-key", firstEnv("GAMEVISION_API_KEY", "OPENAI_API_KEY"), "optional bearer token")
	model := fs.String("model", "", "model id or unique substring")
	timeout := fs.Duration("timeout", 15*time.Minute, "how long to wait for a load or llama-server restart")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if name == "" {
		name = *model
	}

	c := inference.New(inference.Config{BaseURL: *baseURL, APIKey: *apiKey, Timeout: *timeout})
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	switch cmd {
	case "list":
		return printModels(ctx, c)
	case "load":
		if name == "" {
			return fmt.Errorf("usage: gamevision models load <name>")
		}
		return c.LoadModel(ctx, name)
	case "unload":
		if name == "" {
			fmt.Println("unloading currently loaded models")
			return c.UnloadLoaded(ctx)
		}
		return c.UnloadModel(ctx, name)
	case "switch", "use":
		if name == "" {
			return fmt.Errorf("usage: gamevision models switch <name>")
		}
		info, err := c.Switch(ctx, name)
		if err != nil {
			return err
		}
		fmt.Printf("loaded %s (%s)\n", info.ID, info.Status)
		return nil
	default:
		usageModels()
		return fmt.Errorf("unknown models command %q (list, load, unload, switch)", cmd)
	}
}

func printModels(ctx context.Context, c *inference.Client) error {
	router := c.HasRouterLoad(ctx)
	if props, err := c.Props(ctx); err == nil && props.Alias != "" {
		mode := "single-model llama-server (no POST /models/load)"
		if router {
			mode = "llama.cpp router (POST /models/load)"
		}
		fmt.Printf("mode     %s\n", mode)
		fmt.Printf("alias    %s\n", props.Alias)
		fmt.Printf("vision   %v\n", props.Vision)
		if props.Path != "" {
			fmt.Printf("path     %s\n", props.Path)
		}
		if !router {
			fmt.Println()
			fmt.Println("switch <vlm> stops qwen38-solo and starts llama-server with that GGUF.")
			fmt.Println("restore the coding model with: gamevision models switch qwen3.8-27b")
		}
		fmt.Println()
	}
	models, err := c.ListModels(ctx)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		fmt.Println("no models reported")
		return nil
	}
	for _, m := range models {
		status := m.Status
		if status == "" {
			status = "loaded"
		}
		fmt.Printf("%-10s %s\n", status, m.ID)
	}
	return nil
}

func usageModels() {
	fmt.Fprintf(os.Stderr, `Usage:
  gamevision models
  gamevision models switch Qwen3-VL-4B-Instruct
  gamevision models switch qwen3.8-27b
  gamevision models load Qwen3-VL-2B-Instruct
  gamevision models unload
  gamevision models unload Qwen3-VL-2B-Instruct

On llama.cpp router mode, switch unloads VRAM then POST /models/load.

On this machine port 8002 is single-model llama-server (qwen3.8-27b).
switch restarts that process. That stops the coding model until you restore it:
  gamevision models switch qwen3.8-27b
`)
}
