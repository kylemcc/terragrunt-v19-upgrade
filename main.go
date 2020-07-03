package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"

	"github.com/genuinetools/pkg/cli"
	"github.com/hashicorp/hcl/hcl/parser"
	"github.com/kylemcc/terragrunt-v19-upgrade/version"
)

func main() {
	p := cli.NewProgram()
	p.Name = "terragrunt-v12-upgrade"
	p.Version = version.Version
	p.GitCommit = version.GitCommit
	p.Description = `A tool for upgrading terragrunt configs from terragrunt <= v0.18 to >= v0.19`

	p.Action = run
	p.Run()
}

func run(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return flag.ErrHelp
	}

	b, err := ioutil.ReadFile(args[0])
	if err != nil {
		return err
	}

	p, err := parser.Parse(b)
	if err != nil {
		return fmt.Errorf("error parsing file: %v", err)
	}

	pb, _ := json.MarshalIndent(p, "    ", "    ")

	fmt.Printf("p: %s\n", pb)
	return nil
}
