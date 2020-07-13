// Copyright 2020 Kyle McCullough. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kylemcc/terragrunt-v19-upgrade/version"

	"github.com/genuinetools/pkg/cli"
	hclv1ast "github.com/hashicorp/hcl/hcl/ast"
	hclv1parser "github.com/hashicorp/hcl/hcl/parser"
	hclv1token "github.com/hashicorp/hcl/hcl/token"
	hclv2 "github.com/hashicorp/hcl/v2"
	hclv2parse "github.com/hashicorp/hcl/v2/hclparse"
	hclv2syntax "github.com/hashicorp/hcl/v2/hclsyntax"
	hclv2write "github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/hashicorp/terraform/tfdiags"
)

const name = "terragrunt-v19-upgrade"

var (
	tokNewline = &hclv2write.Token{
		Type:  hclv2syntax.TokenNewline,
		Bytes: []byte{'\n'},
	}

	tokComma = &hclv2write.Token{
		Type:  hclv2syntax.TokenComma,
		Bytes: []byte{','},
	}

	tokOBrace = &hclv2write.Token{
		Type:  hclv2syntax.TokenOBrace,
		Bytes: []byte{'{'},
	}

	tokCBrace = &hclv2write.Token{
		Type:  hclv2syntax.TokenCBrace,
		Bytes: []byte{'}'},
	}

	tokOBracket = &hclv2write.Token{
		Type:  hclv2syntax.TokenOBrack,
		Bytes: []byte{'['},
	}

	tokCBracket = &hclv2write.Token{
		Type:  hclv2syntax.TokenCBrack,
		Bytes: []byte{']'},
	}

	tokOQuote = &hclv2write.Token{
		Type:  hclv2syntax.TokenOQuote,
		Bytes: []byte{'"'},
	}

	tokCQuote = &hclv2write.Token{
		Type:  hclv2syntax.TokenCQuote,
		Bytes: []byte{'"'},
	}

	tokEqual = &hclv2write.Token{
		Type:  hclv2syntax.TokenEqual,
		Bytes: []byte{'='},
	}
)

var errNotTerragruntConfig = errors.New("file does not contain a terragrunt attribute")

type command struct {
	recursive bool
	gitMv     bool
	dryRun    bool
	keepOld   bool
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
	p.FlagSet.BoolVar(&cmd.gitMv, "m", false, "Update files in place and \"git mv terraform.tfvars terragrunt.hcl\"")
	p.FlagSet.BoolVar(&cmd.gitMv, "git-mv", false, "Update files in place and \"git mv terraform.tfvars terragrunt.hcl\"")
	p.FlagSet.BoolVar(&cmd.dryRun, "d", false, "Do not update any files, just print changes to stdout")
	p.FlagSet.BoolVar(&cmd.dryRun, "dry-run", false, "Do not update any files, just print changes to stdout")
	p.FlagSet.BoolVar(&cmd.keepOld, "k", false, "Keep old terraform.tfvars files")
	p.FlagSet.BoolVar(&cmd.keepOld, "keep", false, "Keep old terraform.tfvars files")

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
		orig, err := c.readFile(p)
		if err != nil {
			return err
		}

		upgraded, err := c.upgrade(orig)
		if err == errNotTerragruntConfig {
			fmt.Fprintf(os.Stderr, "warning: ignoring file %s. file does not contain a terragrunt attribute.", p)
			continue
		} else if err != nil {
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
		fmt.Fprintf(os.Stderr, "usage: %s [flags] [file|dir ...|-]\n\n", name)
		return flag.ErrHelp
	}

	if len(args) == 1 && args[0] == "-" {
		return nil
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
		if p == "-" {
			return []string{"-"}, nil
		}

		fi, err := os.Stat(p)
		if err != nil {
			return files, err
		}

		if fi.IsDir() {
			if !c.recursive {
				fmt.Fprintf(os.Stderr, "warning: recursive option not specified. ignoring directory %s\n", p)
				continue
			}

			err := filepath.Walk(p, func(path string, fi os.FileInfo, err error) error {
				if err != nil {
					return err
				}

				if fi.IsDir() && fi.Name() == ".terragrunt-cache" {
					return filepath.SkipDir
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

func (c *command) readFile(path string) ([]byte, error) {
	if path == "-" {
		return ioutil.ReadAll(os.Stdin)
	}
	return ioutil.ReadFile(path)
}

// upgrade reads in a terragrunt <= 0.18 config (hcl v1 syntax) and returns
// and upgraded terragrunt >= 0.19 configuration in hcl v2 syntax.
func (c *command) upgrade(input []byte) ([]byte, error) {
	res, err := hclv1parser.Parse(input)
	if err != nil {
		return nil, fmt.Errorf("error parsing file: %v", err)
	}

	var (
		tgSettings []*hclv1ast.ObjectItem
		inputVars  []*hclv1ast.ObjectItem
	)

	detachedComments := c.loadDetachedComments(res)

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

	if len(tgSettings) == 0 {
		return nil, errNotTerragruntConfig
	}

	f := hclv2write.NewEmptyFile()
	body := f.Body()

	c.writeNode(-1, "", body, &hclv1ast.ObjectList{Items: tgSettings}, detachedComments)

	if len(inputVars) > 0 {
		body.AppendNewline()
		inputs := &hclv1ast.ObjectItem{
			Keys: []*hclv1ast.ObjectKey{
				{
					Token: hclv1token.Token{
						Type: hclv1token.IDENT,
						Pos:  inputVars[0].Pos(),
						Text: "inputs",
					},
				},
			},
			Val: &hclv1ast.ObjectType{
				List: &hclv1ast.ObjectList{
					Items: inputVars,
				},
			},
		}

		c.writeNode(-1, "", body, inputs, detachedComments)
	}

	if detachedComments.Len() > 0 {
		// write out any remaining comments
		for _, cg := range *detachedComments {
			body.AppendNewline()
			c.writeNode(0, "", body, cg, nil)
		}
	}

	return hclv2write.Format(f.Bytes()), nil
}

func (c *command) writeNode(depth int, parentKey string, body *hclv2write.Body, node hclv1ast.Node, cl *commentList) {
	// write out any detached comments that should come before the current node
	if cl != nil {
		comments := cl.PopBefore(node.Pos())
		c.writeComments(body, comments)
	}

	switch nv := node.(type) {
	case *hclv1ast.ListType:
		oneline := nv.Lbrack.Line == nv.Rbrack.Line
		body.AppendUnstructuredTokens(hclv2write.Tokens{tokOBracket})
		if !oneline {
			body.AppendNewline()
		}

		for i, n := range nv.List {
			if i > 0 {
				body.AppendUnstructuredTokens(hclv2write.Tokens{tokComma})
				if !oneline {
					body.AppendNewline()
				}
			}
			c.writeNode(depth+1, parentKey, body, n, cl)
		}

		if !oneline {
			// if it's not a single-line list, add a trailing comma
			body.AppendUnstructuredTokens(hclv2write.Tokens{tokComma, tokNewline})
		}

		body.AppendUnstructuredTokens(hclv2write.Tokens{tokCBracket})
	case *hclv1ast.LiteralType:
		if nv.LeadComment != nil {
			c.writeNode(depth, parentKey, body, nv.LeadComment, nil)
		}

		c.writeLiteral(body, nv)

		if nv.LineComment != nil {
			c.writeNode(depth, parentKey, body, nv.LineComment, nil)
		}
	case *hclv1ast.ObjectItem:
		if nv.LeadComment != nil {
			c.writeNode(depth, parentKey, body, nv.LeadComment, nil)
		}

		key := nv.Keys[0].Token.Text
		tok := hclv2write.Tokens{
			{Type: hclv2syntax.TokenIdent, Bytes: []byte(key)},
		}

		if !isBlock(key, depth, parentKey) {
			tok = append(tok, tokEqual)
		} else if len(nv.Keys) > 1 {
			for _, k := range nv.Keys[1:] {
				kv := k.Token.Value().(string)
				tok = append(tok, tokOQuote, &hclv2write.Token{Type: hclv2syntax.TokenQuotedLit, Bytes: []byte(kv)}, tokCQuote)
			}
		}

		body.AppendUnstructuredTokens(tok)
		c.writeNode(depth, key, body, nv.Val, cl)

		if nv.LineComment != nil {
			c.writeNode(depth, parentKey, body, nv.LineComment, nil)
		}

		body.AppendNewline()
	case *hclv1ast.ObjectList:
		for i, item := range nv.Items {
			if i > 0 && needNewline(item, nv.Items[i-1], cl) {
				body.AppendNewline()
			}
			c.writeNode(depth+1, parentKey, body, item, cl)
		}
	case *hclv1ast.ObjectType:
		body.AppendUnstructuredTokens(hclv2write.Tokens{tokOBrace, tokNewline})
		c.writeNode(depth, parentKey, body, nv.List, cl)
		body.AppendUnstructuredTokens(hclv2write.Tokens{tokCBrace})
	case *hclv1ast.CommentGroup:
		for _, c := range nv.List {
			body.AppendUnstructuredTokens(hclv2write.Tokens{
				tokComment(c.Text),
				tokNewline,
			})
		}
	}
}

func (c *command) writeComments(body *hclv2write.Body, comments commentList) {
	if len(comments) == 0 {
		return
	}

	for _, cg := range comments {
		if c := cg.List[0]; c.Start.Line != 1 {
			// Don't prepend a newline if this comment is the first thing in the file
			body.AppendNewline()
		}

		for _, c := range cg.List {
			body.AppendUnstructuredTokens(hclv2write.Tokens{
				{
					Type:  hclv2syntax.TokenComment,
					Bytes: []byte(c.Text),
				},
				tokNewline,
			})
		}
	}

	body.AppendNewline()
}

func (c *command) writeLiteral(body *hclv2write.Body, val *hclv1ast.LiteralType) {
	switch val.Token.Type {
	case hclv1token.NUMBER, hclv1token.FLOAT:
		body.AppendUnstructuredTokens(hclv2write.Tokens{
			{
				Type:  hclv2syntax.TokenNumberLit,
				Bytes: []byte(val.Token.Text),
			},
		})
	case hclv1token.BOOL:
		body.AppendUnstructuredTokens(hclv2write.Tokens{
			{
				Type:  hclv2syntax.TokenIdent,
				Bytes: []byte(val.Token.Text),
			},
		})
	case hclv1token.HEREDOC:
		// TODO: a quick look at the terraform 0.12upgrade command indicates
		// that this may not be sufficient. I should probably insert a TODO
		// into the upgraded configuration to check any upgraded heredocs.
		// This is good enough for now though.

		newlineIdx := strings.IndexByte(val.Token.Text, '\n')

		if newlineIdx < 0 {
			panic("invalid heredoc")
		}

		// start from 2; don't include <<
		delim := val.Token.Text[2 : newlineIdx+1]
		if delim[0] == '-' {
			delim = delim[1:]
		}

		body.AppendUnstructuredTokens(hclv2write.Tokens{
			{
				Type:  hclv2syntax.TokenOHeredoc,
				Bytes: append([]byte("<<"), []byte(delim)...),
			},
			{
				Type:  hclv2syntax.TokenStringLit,
				Bytes: []byte(val.Token.Value().(string)),
			},
			{
				Type:  hclv2syntax.TokenCHeredoc,
				Bytes: []byte(delim),
			},
		})
	case hclv1token.STRING:
		tmpTok := upgradeExpr(val.Token.Text)

		// convert from hclsyntax.Tokens to hclwrite.Tokens
		var tok hclv2write.Tokens
		for _, t := range tmpTok {
			tok = append(tok, &hclv2write.Token{
				Type:  t.Type,
				Bytes: t.Bytes,
			})
		}

		upgradeFunctionNames(tok)
		body.AppendUnstructuredTokens(tok)
	}
}

// loadDetachedComments returns comments that are not associated with a node as either
// a lead comment or a line comment.
func (c *command) loadDetachedComments(f *hclv1ast.File) *commentList {
	var (
		ret commentList
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

	return &ret
}

func (c *command) save(path string, contents []byte) error {
	// check the new config
	p := hclv2parse.NewParser()
	_, diags := p.ParseHCL(contents, path)
	if diags.HasErrors() {
		var d tfdiags.Diagnostics
		d = d.Append(diags)
		return d.Err()
	}

	if c.dryRun {
		fmt.Printf("%s:\n%s\n", path, contents)
		return nil
	} else if path == "-" {
		os.Stdout.Write(contents)
		return nil
	}

	base := filepath.Dir(path)
	newPath := filepath.Join(base, "terragrunt.hcl")
	if c.gitMv {
		// update the source file and git mv it
		err := ioutil.WriteFile(path, contents, 0644)
		if err != nil {
			return err
		}
		fmt.Printf("Updated %s\n", path)

		cmd := exec.Command("git", "mv", path, newPath)
		if err := cmd.Run(); err != nil {
			return err
		}
	} else {
		err := ioutil.WriteFile(newPath, contents, 0644)
		if err != nil {
			return err
		}
		fmt.Printf("Updated %s\n", path)

		if !c.keepOld {
			return os.Remove(path)
		}
	}

	return nil
}

type commentList []*hclv1ast.CommentGroup

func (cl *commentList) Len() int {
	return len(*cl)
}

func (cl *commentList) PeekBefore(pos hclv1token.Pos) commentList {
	var i int

	for i = 0; i < len(*cl); i++ {
		if (*cl)[i].Pos().After(pos) {
			break
		}
	}

	if i == 0 {
		return nil
	}
	return (*cl)[:i]
}

func (cl *commentList) PopBefore(pos hclv1token.Pos) commentList {
	var (
		ret commentList
		i   int
	)

	for i = 0; i < len(*cl); i++ {
		if (*cl)[i].Pos().After(pos) {
			break
		}
	}

	if i == 0 {
		return nil
	}

	ret = (*cl)[:i]
	*cl = (*cl)[i:]
	return ret
}

func tokComment(text string) *hclv2write.Token {
	return &hclv2write.Token{
		Type:  hclv2syntax.TokenComment,
		Bytes: []byte(text),
	}
}

var topLevelBlocks = []string{"terraform", "remote_state", "include", "dependencies"}

// isBlock returns a boolean indicating whether the node identified by
// key at the given depth under the specified parent should be a block.
// If this returns false, the node should be an attribute.
func isBlock(key string, depth int, parent string) bool {
	if depth == 0 {
		for _, k := range topLevelBlocks {
			if key == k {
				return true
			}
		}
	} else if depth == 1 && parent == "terraform" && key == "extra_arguments" {
		return true
	}

	return false
}

// needNewline returns true if an extra newline is needed between
// nodes. These cases include:
//   - The current node has a leading comment
//   - The current or previous node is an object
//   - The current or previous node is a multiline list
// But not if:
//   - The previous element had a line comment
//   - There is a detached comment after the previous node
func needNewline(curr, prev *hclv1ast.ObjectItem, cl *commentList) bool {
	if prev.LineComment != nil {
		// The previous line comment includes a newline
		return false
	} else if c := cl.PeekBefore(curr.Pos()); len(c) > 0 {
		// The previous detached comment includes a newline
		return false
	} else if curr.LeadComment != nil {
		return true
	}

	switch v := curr.Val.(type) {
	case *hclv1ast.LiteralType:
		if v.LeadComment != nil {
			return true
		}
	case *hclv1ast.ListType:
		if v.Lbrack.Line != v.Rbrack.Line {
			return true
		}
	case *hclv1ast.ObjectType:
		return true
	}

	switch v := prev.Val.(type) {
	case *hclv1ast.ListType:
		if v.Lbrack.Line != v.Rbrack.Line {
			return true
		}
	case *hclv1ast.ObjectType:
		return true
	}

	return false
}

func upgradeExpr(expr string) hclv2syntax.Tokens {
	tok, diag := hclv2syntax.LexExpression([]byte(expr), "", hclv2.Pos{})
	if diag.HasErrors() {
		// TODO: should probably do something about this.
		return tok
	}

	if tok[len(tok)-1].Type == hclv2syntax.TokenEOF {
		tok = tok[:len(tok)-1]
	}

	if len(tok) < 5 {
		// Not enough tokens for an interpolation (open quote, start template (${), inner token(s), close template (}), close quote)
		return tok
	}

	oq := tok[0]
	ot := tok[1]
	ct := tok[len(tok)-2]
	cq := tok[len(tok)-1]
	inner := tok[2 : len(tok)-2]

	if oq.Type != hclv2syntax.TokenOQuote || ot.Type != hclv2syntax.TokenTemplateInterp || ct.Type != hclv2syntax.TokenTemplateSeqEnd || cq.Type != hclv2syntax.TokenCQuote {
		// Not an intepolation that looks like "${expr}"
		return tok
	}

	quotes := 0
	for _, t := range inner {
		if t.Type == hclv2syntax.TokenOQuote {
			quotes++
			continue
		}
		if t.Type == hclv2syntax.TokenCQuote {
			quotes--
			continue
		}
		if quotes > 0 {
			// Nested interpolations are ok
			continue
		}
		if t.Type == hclv2syntax.TokenTemplateInterp {
			// Interpolation outside of a string, e.g., ${expr1}${expr2}
			return tok
		}
	}

	// Return the tokens without the ${}
	return inner
}

var renameFuncs = map[string]string{
	"get_tfvars_dir":        "get_terragrunt_dir",
	"get_parent_tfvars_dir": "get_parent_terragrunt_dir",
}

func upgradeFunctionNames(tokens hclv2write.Tokens) {
	for i, t := range tokens {
		if t.Type == hclv2syntax.TokenIdent {
			newName, ok := renameFuncs[string(t.Bytes)]
			if !ok {
				continue
			}

			if i+2 >= len(tokens)-1 {
				// need at least 2 more tokens in the expresion, '(' and ')', for this to be a valid function call
				// since we don't have enough, continue
				continue
			}

			// finally, make sure the next 2 tokens actually _are_ '(' and ')' - since the 2
			// renamed functions don't accept any arguments
			if tokens[i+1].Type != hclv2syntax.TokenOParen || tokens[i+2].Type != hclv2syntax.TokenCParen {
				continue
			}

			t.Bytes = []byte(newName)
		}
	}
}
