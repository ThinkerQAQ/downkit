package devcontrol

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	contract "github.com/thinkerqaq/devtool/sdk/contract"
	environment "github.com/thinkerqaq/devtool/sdk/environment"
	extension "github.com/thinkerqaq/devtool/sdk/extension"
	portable "github.com/thinkerqaq/devtool/sdk/portable"
	"github.com/thinkerqaq/devtool/sdk/project"
)

type Provider struct{}

func (p *Provider) ExtensionDescriptor() extension.Descriptor {
	return extension.Descriptor{
		ID: "project.downkit", Kind: extension.KindProject,
		Requires: []string{environment.ServiceName, portable.ServiceName},
	}
}

func (p *Provider) ProjectDescriptor() contract.ProjectDescriptor {
	commands := []contract.CommandDescriptor{
		{ID: "build", Title: "Build", Description: "Build DownKit Go Bridge through the Go maintenance tool.", SideEffect: contract.SideEffectWrite},
		{ID: "verify", Title: "Verify", Description: "Run Go and extension checks from the Go maintenance tool.", SideEffect: contract.SideEffectRead},
		{ID: "package", Title: "Release Package", Description: "Build five cross-platform archives, manifests and checksums.", SideEffect: contract.SideEffectWrite},
		{ID: "components.manifest", Title: "Component Manifest", Description: "Generate checksummed installed-component metadata.", SideEffect: contract.SideEffectWrite},
		{ID: "components.install", Title: "Install Component", Description: "Install pinned whisper/llama upstream binaries after SHA verification.", SideEffect: contract.SideEffectWrite},
		{ID: "android.bind", Title: "Android Go Binding", Description: "Build the Go AAR using gomobile and Android NDK.", SideEffect: contract.SideEffectWrite},
		{ID: "android.build", Title: "Android APK", Description: "Build the Go AAR and Android APK through Gradle.", SideEffect: contract.SideEffectWrite},
		{ID: "ffmpeg.native", Title: "Native FFmpeg", Description: "Use official FFmpeg configure/make with the scoped codec list.", SideEffect: contract.SideEffectWrite},
		{ID: "ffmpeg.android", Title: "Android FFmpeg", Description: "Build minimal FFmpeg JNI libraries with Android NDK.", SideEffect: contract.SideEffectWrite},
		{ID: "icons.generate", Title: "Icon Assets", Description: "Generate browser PNG sizes with Go image codecs.", SideEffect: contract.SideEffectWrite},
		{ID: "native.install", Title: "Native Host Install", Description: "Register browser Native Messaging through the Go installer.", SideEffect: contract.SideEffectWrite},
		{ID: "native.uninstall", Title: "Native Host Uninstall", Description: "Remove browser registration without deleting downloads.", SideEffect: contract.SideEffectWrite},
		{ID: "deploy", Title: "Deploy", Description: "Install Bridge and extension to an explicitly chosen directory without changing Data.", SideEffect: contract.SideEffectWrite},
		{ID: "runtime.doctor", Title: "Runtime Doctor", Description: "Inspect configured portable runtime.", SideEffect: contract.SideEffectRead},
	}
	resources := []contract.ResourceDescriptor{
		{ID: "downkit-bridge", Title: "Desktop Bridge", Description: "Native Go Bridge executable"},
		{ID: "downkit-release", Title: "Release Assets", Description: "Cross-platform archives and SHA256SUMS"},
	}
	actions := make([]contract.ActionDescriptor, 0, len(commands))
	for _, command := range commands {
		if command.ID == "runtime.doctor" {
			continue
		}
		actions = append(actions, contract.ActionDescriptor{CommandID: command.ID, Label: command.Title})
	}
	return contract.ProjectDescriptor{
		Identity:   contract.ProjectIdentity{Name: "DownKit"},
		Commands:   commands,
		Resources:  resources,
		Views:      []contract.ViewDescriptor{{ID: "development", Title: "Development", Resources: []string{"downkit-bridge", "downkit-release"}, Actions: actions}},
		Navigation: []contract.NavigationItem{{ID: "development", Title: "Development", ViewID: "development"}},
	}
}

func (p *Provider) Execute(ctx project.Context, command string, args map[string]any) error {
	workspace, err := os.Getwd()
	if err != nil {
		return err
	}
	if command == "runtime.doctor" {
		var response portable.DoctorResponse
		if err := ctx.InvokeService(portable.ServiceName, portable.MethodDoctor, struct{}{}, &response); err != nil {
			return err
		}
		return ctx.Emit("result", fmt.Sprintf("%s READY: %s (%s)", response.Provider, response.Version, response.Smoke))
	}
	aliases := map[string]string{
		"build": "build", "verify": "verify", "package": "package",
		"components.manifest": "components", "components.install": "tool-install",
		"android.bind": "android-bind", "android.build": "android-build",
		"ffmpeg.native": "ffmpeg-native", "ffmpeg.android": "ffmpeg-android",
		"icons.generate": "icons", "native.install": "install", "native.uninstall": "uninstall",
		"deploy": "deploy",
	}
	operation, ok := aliases[command]
	if !ok {
		return fmt.Errorf("unknown DownKit command: %s", command)
	}
	arguments := []string{"-C", "tools/downkitdev", "run", ".", operation}
	for _, option := range []struct{ arg, key string }{
		{"--root", "root"},
		{"--os", "os"}, {"--arch", "arch"}, {"--output", "output"}, {"--target", "target"},
		{"--source", "source"}, {"--archive", "archive"}, {"--version", "version"}, {"--extension-id", "extensionID"},
	} {
		value, exists := args[option.key]
		if !exists || value == nil {
			continue
		}
		textValue, ok := value.(string)
		if !ok || strings.TrimSpace(textValue) == "" {
			continue
		}
		arguments = append(arguments, option.arg, textValue)
	}
	if args["allowMissingTools"] == true {
		arguments = append(arguments, "--allow-missing-tools")
	}
	if command == "components.install" {
		toolName, ok := args["tool"].(string)
		if !ok || toolName == "" {
			return fmt.Errorf("components.install requires tool=whisper or llama")
		}
		arguments = append(arguments, toolName)
	}
	if command == "deploy" {
		if _, ok := args["target"].(string); !ok {
			return fmt.Errorf("deploy requires explicit target")
		}
	}
	if command == "package" {
		// Production packaging can fail if pinned Windows sidecars are missing;
		// this is intentional, not a reason to silently ship incomplete bundles.
		if err := os.MkdirAll(filepath.Join(workspace, ".devtool", "artifacts"), 0o755); err != nil {
			return err
		}
	}
	var result environment.RunResult
	if err := ctx.InvokeService(environment.ServiceName, environment.MethodRun,
		environment.CommandRequest{Root: workspace, Executable: "go", Args: arguments}, &result); err != nil {
		return err
	}
	if value := strings.TrimSpace(result.Stdout); value != "" {
		if err := ctx.Emit("result", value); err != nil {
			return err
		}
	}
	if value := strings.TrimSpace(result.Stderr); value != "" {
		if err := ctx.Emit("result", value); err != nil {
			return err
		}
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("DownKit %s failed: exit code %d", command, result.ExitCode)
	}
	return nil
}
