package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/Acosmi/ClawAcosmi/cmd/desktop/internal/release"
)

func main() {
	var manifestPath string
	var srcRoot string
	var outRoot string

	flag.StringVar(&manifestPath, "manifest", "", "path to skills package manifest")
	flag.StringVar(&srcRoot, "src", "", "source docs/skills root")
	flag.StringVar(&outRoot, "out", "", "destination docs/skills root in the app bundle")
	flag.Parse()

	if manifestPath == "" || srcRoot == "" || outRoot == "" {
		fmt.Fprintln(os.Stderr, "usage: stage_skills -manifest <file> -src <docs/skills> -out <bundle/docs/skills>")
		os.Exit(2)
	}

	if err := release.StageSkillsPackage(srcRoot, outRoot, manifestPath); err != nil {
		fmt.Fprintf(os.Stderr, "stage skills: %v\n", err)
		os.Exit(1)
	}
}
