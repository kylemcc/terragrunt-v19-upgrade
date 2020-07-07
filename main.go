// Copyright 2020 Kyle McCullough. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/kylemcc/terragrunt-v19-upgrade/version"

	"github.com/genuinetools/pkg/cli"
	hclv1ast "github.com/hashicorp/hcl/hcl/ast"
	hclv1parser "github.com/hashicorp/hcl/hcl/parser"
	hclv1token "github.com/hashicorp/hcl/hcl/token"
)

const name = "terragrunt-v12-upgrade"

type command struct {
	recursive bool
	gitMv     bool
	dryRun    bool
}

func main() {
	p := cli.NewProgram()
	p.Name = name
	p.Version = version.Version
	p.GitCommit = version.GitCommit
	p.Description = `A tool for upgrading terragrunt configs from terragrunt <= v0.18 to >= v0.19`

	var cmd command

	p.FlagSet = flag.NewFlagSet("global", flag.ExitOnError)
	p.FlagSet.BoolVar(&cmd.recursive, "r", false, "Search subdirectores for terraform.tfvars files")
	p.FlagSet.BoolVar(&cmd.recursive, "recursive", false, "Search subdirectores for terraform.tfvars files")
	p.FlagSet.BoolVar(&cmd.gitMv, "git-mv", false, "Update files in place and \"git mv terraform.tfvars terragrunt.hcl\"")
	p.FlagSet.BoolVar(&cmd.dryRun, "dry-run", false, "Do not update any files, just print changes to stdout")

	p.Action = cmd.run
	p.Run()
}

func (c *command) run(ctx context.Context, args []string) error {
	if err := c.validateArgs(args); err != nil {
		return err
	}

	paths, err := c.loadFiles(args)
	if err != nil {
		return err
	}

	for _, p := range paths {
		orig, err := ioutil.ReadFile(p)
		if err != nil {
			return err
		}

		upgraded, err := c.upgrade(orig)
		if err != nil {
			return fmt.Errorf("error upgrading file %s: %v", p, err)
		}

		if err := c.save(p, upgraded); err != nil {
			return err
		}
	}

	return nil
}

func (c *command) validateArgs(args []string) error {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "usage: %s [flags] [file|dir ...]\n\n", name)
		return flag.ErrHelp
	}

	for _, p := range args {
		fi, err := os.Stat(p)
		if err != nil {
			return err
		}

		if fi.IsDir() && !c.recursive {
			fmt.Fprintf(os.Stderr, "error: %s is a directory\n\n", p)
			return flag.ErrHelp
		}
	}

	return nil
}

func (c *command) loadFiles(args []string) ([]string, error) {
	var files []string

	for _, p := range args {
		fi, err := os.Stat(p)
		if err != nil {
			return files, err
		}

		if fi.IsDir() {
			if !c.recursive {
				fmt.Fprintf(os.Stderr, "warning: recursive option to specified. ignoring directory %s\n", p)
				continue
			}

			err := filepath.Walk(p, func(path string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				if fi.Name() == "terraform.tfvars" {
					files = append(files, path)
				}
				return nil
			})

			if err != nil {
				return files, err
			}
		} else {
			if fi.Name() != "terraform.tfvars" {
				fmt.Fprintf(os.Stderr, "warning: ignoring file %s", p)
				continue
			}
			files = append(files, p)
		}
	}

	return files, nil
}

// upgrade reads in a terragrunt <= 0.18 config (hcl v1 syntax) and returns
// and upgraded terragrunt >= 0.19 configuration in hcl v2 syntax.
func (c *command) upgrade(input []byte) ([]byte, error) {
	res, err := hclv1parser.Parse(input)
	if err != nil {
		return nil, fmt.Errorf("error parsing file: %v", err)
	}

	var (
		_          bytes.Buffer
		tgSettings []hclv1ast.Node
		inputVars  []*hclv1ast.ObjectItem
	)

	_ = c.loadDetachedComments(res)

	root := res.Node.(*hclv1ast.ObjectList)
	for _, item := range root.Items {
		item := item
		if item.Keys[0].Token.Text == "terragrunt" {
			obj := item.Val.(*hclv1ast.ObjectType)
			for _, o := range obj.List.Items {
				tgSettings = append(tgSettings, o)
			}
		} else {
			inputVars = append(inputVars, item)
		}
	}

	return nil, nil
}

// loadDetachedComments returns comments that are not associated with a node as either
// a lead comment or a line comment.
func (c *command) loadDetachedComments(f *hclv1ast.File) []*hclv1ast.CommentGroup {
	var (
		ret []*hclv1ast.CommentGroup
		m   = make(map[hclv1token.Pos]*hclv1ast.CommentGroup)
	)

	hclv1ast.Walk(f, func(n hclv1ast.Node) (hclv1ast.Node, bool) {
		switch val := n.(type) {
		case *hclv1ast.ObjectItem:
			if val.LeadComment != nil {
				m[val.LeadComment.Pos()] = val.LeadComment
			}

			if val.LineComment != nil {
				m[val.LineComment.Pos()] = val.LineComment
			}
		case *hclv1ast.LiteralType:
			if val.LeadComment != nil {
				m[val.LeadComment.Pos()] = val.LeadComment
			}

			if val.LineComment != nil {
				m[val.LineComment.Pos()] = val.LineComment
			}
		}

		return n, true
	})

	for _, c := range f.Comments {
		c := c
		if _, ok := m[c.Pos()]; !ok {
			ret = append(ret, c)
		}
	}

	return ret
}

func (c *command) save(path string, contents []byte) error {
	return nil
}
