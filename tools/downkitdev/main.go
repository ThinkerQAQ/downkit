package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ThinkerQAQ/downkit/tools/downkitdev/internal/maint"
)

func main() {
	if err := execute(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "downkitdev:", err)
		os.Exit(1)
	}
}

func execute(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: downkitdev <verify|build|package|deploy|install|uninstall|components|tool-install|android-bind|android-build|ffmpeg-native|ffmpeg-android|icons> [flags]")
	}
	command := args[0]
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	rootFlag := flags.String("root", "", "repository or installed DownKit root")
	output := flags.String("output", "", "output path or destination directory")
	version := flags.String("version", "", "release version (defaults to extension manifest)")
	platform := flags.String("os", runtime.GOOS, "target operating system")
	arch := flags.String("arch", runtime.GOARCH, "target architecture")
	target := flags.String("target", "", "explicit install/deploy directory")
	extensionID := flags.String("extension-id", "hfjpenjlamneepmigemmmiibebpokmik", "Chromium extension ID")
	sourceArchive := flags.String("archive", "", "local verified third-party archive path")
	ffmpegSource := flags.String("source", "", "FFmpeg source directory or release source archive")
	allowMissing := flags.Bool("allow-missing-tools", false, "allow explicitly incomplete development-only tool bundle")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	root := strings.TrimSpace(*rootFlag)
	if root == "" {
		var err error
		root, err = maint.FindRoot()
		if err != nil {
			return err
		}
	} else {
		absolute, err := filepath.Abs(root)
		if err != nil {
			return err
		}
		root = absolute
	}
	switch command {
	case "verify":
		return maint.Verify(root)
	case "build":
		return maint.Build(root, *platform, *arch, *output)
	case "package":
		return maint.Package(root, *version, *allowMissing, *ffmpegSource)
	case "deploy":
		if *target == "" {
			return errors.New("deploy requires --target to prevent unintended installation changes")
		}
		return maint.Deploy(root, *target, *platform, *arch)
	case "install":
		return maint.Install(root, *extensionID)
	case "uninstall":
		return maint.Uninstall(root)
	case "components":
		return maint.Components(root, *platform, *arch, *output)
	case "tool-install":
		if flags.NArg() < 1 {
			return errors.New("tool-install needs whisper or llama")
		}
		return maint.InstallTool(root, flags.Arg(0), *sourceArchive)
	case "android-bind":
		return maint.AndroidBind(root)
	case "android-build":
		return maint.AndroidBuild(root)
	case "ffmpeg-native":
		return maint.FFmpegNative(root, *ffmpegSource, *output, *version)
	case "ffmpeg-android":
		return maint.FFmpegAndroid(root, *ffmpegSource, *output)
	case "icons":
		return maint.Icons(root)
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}
