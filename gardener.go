/*
Copyright 2016 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	buildifier "github.com/bazelbuild/buildifier/core"
)

const srcsTargetName = "srcs"
const recursiveSrcsTargetName = "recursive-srcs"
const automanagedTag = "automanaged"

func walkTree(root string) ([]string, error) {
	files, err := ioutil.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var children []string = nil
	buildFile := ""
	for _, f := range files {
		if f.IsDir() {
			c, err := walkTree(filepath.Join(root, f.Name()))
			if err != nil {
				return nil, err
			}
			if c != nil {
				children = append(children, c...)
			}
		}
		if f.Name() == "BUILD" || f.Name() == "BUILD.bazel" {
			buildFile = f.Name()
		}
	}
	if buildFile == "" {
		return children, nil
	}

	retTarget := "srcs"
	//	fmt.Printf("%s: srcs=*\n", root)
	if len(children) > 0 {
		//		fmt.Printf("%s: recursive-srcs=%s\n", root, children)
		retTarget = "recursive-srcs"
	}
	if err := fixBuild(filepath.Join(root, buildFile), children); err != nil {
		return nil, err
	}
	return []string{fmt.Sprintf("//%s:%s", root, retTarget)}, nil
}

func newFilegroupRule(name string, deps []string) *buildifier.Rule {
	rule := buildifier.Rule{
		Call: &buildifier.CallExpr{
			X: &buildifier.LiteralExpr{Token: "filegroup"},
		},
	}
	rule.SetAttr("name", &buildifier.StringExpr{Value: name})
	var l []buildifier.Expr
	for _, d := range deps {
		l = append(l, &buildifier.StringExpr{Value: d})
	}
	rule.SetAttr("srcs", &buildifier.ListExpr{List: l})
	rule.SetAttr("tags", &buildifier.ListExpr{
		List: []buildifier.Expr{
			&buildifier.StringExpr{Value: automanagedTag},
		}})
	return &rule
}

func fixBuild(path string, children []string) error {
	f, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	fmt.Printf("Opening %s\n", path)
	b, err := buildifier.Parse(path, f)
	if err != nil {
		return err
	}

	newRules := make(map[string]*buildifier.Rule)
	newRules[srcsTargetName] = newFilegroupRule(srcsTargetName, nil)
	newRules[srcsTargetName].SetAttr("srcs", &buildifier.LiteralExpr{Token: `glob(["**"])`})

	if len(children) > 0 {
		children = append(children, fmt.Sprintf(":%s", srcsTargetName))
		newRules[recursiveSrcsTargetName] = newFilegroupRule(recursiveSrcsTargetName, children)
	}

	for _, or := range b.Rules("filegroup") {
		nr, found := newRules[or.Name()]
		if !found {
			continue
		}
		if RuleIsManaged(or) {
			or.SetAttr("srcs", nr.Attr("srcs"))
		}
		delete(newRules, or.Name())
	}
	for _, nr := range newRules {
		fmt.Printf("adding %s:%s\n", path, nr.Name())
		b.Stmt = append(b.Stmt, nr.Call)
	}
	out := buildifier.Format(b)
	// TODO: only write if needed
	ioutil.WriteFile(path, out, 0644)
	fmt.Printf("Wrote BUILD file for %s\n", path)

	return nil
}

// TODO: maybe depend on gazel?
func RuleIsManaged(r *buildifier.Rule) bool {
	var automanaged bool
	for _, tag := range r.AttrStrings("tags") {
		if tag == automanagedTag {
			automanaged = true
			break
		}
	}
	return automanaged
}

func main() {
	walkTree(".")
}
