// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"
)

func isGoProfileTemplate(tmpl string) bool {
	return strings.Contains(tmpl, "{{")
}

func compileTrapTemplates(td *TrapDef, fileVarbinds map[string]VarbindDef) error {
	if td == nil {
		return nil
	}

	if isGoProfileTemplate(td.Description) {
		tpl, err := compileProfileTemplate("description", td.Description, td, fileVarbinds, "")
		if err != nil {
			return err
		}
		td.descriptionTemplate = tpl
	}

	for key, tmpl := range td.Labels {
		if !isGoProfileTemplate(tmpl) {
			continue
		}
		tpl, err := compileProfileTemplate("label_"+key, tmpl, td, fileVarbinds, key)
		if err != nil {
			return err
		}
		if td.labelTemplates == nil {
			td.labelTemplates = make(map[string]*template.Template)
		}
		td.labelTemplates[key] = tpl
	}

	return nil
}

func compileProfileTemplate(name, tmpl string, td *TrapDef, fileVarbinds map[string]VarbindDef, labelKey string) (*template.Template, error) {
	src := "<unknown>"
	oid := ""
	if td != nil {
		src = td.sourceFile
		oid = td.OID
	}

	tpl, err := template.New(name).Funcs(validationTemplateFuncMap()).Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("%s: trap entry %s: invalid template %q: %w", src, oid, name, err)
	}

	ctx := templateValidationContext{
		src:          src,
		trapOID:      oid,
		templateName: name,
		labelKey:     labelKey,
		td:           td,
		fileVarbinds: fileVarbinds,
	}
	if err := validateProfileTemplateTree(tpl.Tree.Root, ctx); err != nil {
		return nil, err
	}

	return tpl, nil
}

type templateValidationContext struct {
	src          string
	trapOID      string
	templateName string
	labelKey     string
	td           *TrapDef
	fileVarbinds map[string]VarbindDef
}

func (ctx templateValidationContext) isLabel() bool {
	return ctx.labelKey != ""
}

func (ctx templateValidationContext) errf(format string, args ...any) error {
	return fmt.Errorf("%s: trap entry %s: invalid template %q: %s", ctx.src, ctx.trapOID, ctx.templateName, fmt.Sprintf(format, args...))
}

func validationTemplateFuncMap() template.FuncMap {
	return template.FuncMap{
		"hostname":       func() string { return "" },
		"source_ip":      func() string { return "" },
		"trap_name":      func() string { return "" },
		"vendor":         func() string { return "" },
		"trap_interface": func() string { return "" },
		"trap_neighbors": func() string { return "" },
		"value":          func(string) string { return "" },
		"raw":            func(string) string { return "" },
		"first":          func(...string) string { return "" },
	}
}

func validateProfileTemplateTree(n parse.Node, ctx templateValidationContext) error {
	switch node := n.(type) {
	case nil:
		return nil
	case *parse.ListNode:
		if node == nil {
			return nil
		}
		for _, child := range node.Nodes {
			if err := validateProfileTemplateTree(child, ctx); err != nil {
				return err
			}
		}
		return nil
	case *parse.TextNode:
		return nil
	case *parse.ActionNode:
		if node == nil {
			return nil
		}
		return validateProfileTemplatePipe(node.Pipe, ctx)
	case *parse.WithNode:
		if node == nil {
			return nil
		}
		if err := validateProfileTemplatePipe(node.Pipe, ctx); err != nil {
			return err
		}
		if err := validateProfileTemplateTree(node.List, ctx); err != nil {
			return err
		}
		return validateProfileTemplateTree(node.ElseList, ctx)
	case *parse.IfNode:
		if node == nil {
			return nil
		}
		if err := validateProfileTemplatePipe(node.Pipe, ctx); err != nil {
			return err
		}
		if err := validateProfileTemplateTree(node.List, ctx); err != nil {
			return err
		}
		return validateProfileTemplateTree(node.ElseList, ctx)
	default:
		return ctx.errf("forbidden template action %T", n)
	}
}

func validateProfileTemplatePipe(p *parse.PipeNode, ctx templateValidationContext) error {
	if p == nil {
		return ctx.errf("empty pipeline")
	}
	if len(p.Decl) > 0 || p.IsAssign {
		return ctx.errf("template variables are not allowed")
	}
	if len(p.Cmds) != 1 {
		return ctx.errf("template pipelines are not allowed")
	}
	return validateProfileTemplateCommand(p.Cmds[0], ctx)
}

func validateProfileTemplateCommand(cmd *parse.CommandNode, ctx templateValidationContext) error {
	if cmd == nil || len(cmd.Args) == 0 {
		return ctx.errf("empty command")
	}

	switch first := cmd.Args[0].(type) {
	case *parse.IdentifierNode:
		return validateProfileTemplateFunction(first.Ident, cmd.Args[1:], ctx)
	case *parse.DotNode:
		if len(cmd.Args) != 1 {
			return ctx.errf("dot command does not accept arguments")
		}
		return nil
	case *parse.StringNode:
		if len(cmd.Args) != 1 {
			return ctx.errf("string literal command does not accept arguments")
		}
		return nil
	default:
		return ctx.errf("forbidden command %T", first)
	}
}

func validateProfileTemplateFunction(name string, args []parse.Node, ctx templateValidationContext) error {
	switch name {
	case "hostname", "source_ip", "trap_interface", "trap_neighbors":
		if ctx.isLabel() {
			return ctx.errf("label %q references unbounded function %q", ctx.labelKey, name)
		}
		if len(args) != 0 {
			return ctx.errf("function %q does not accept arguments", name)
		}
		return nil
	case "trap_name", "vendor":
		if len(args) != 0 {
			return ctx.errf("function %q does not accept arguments", name)
		}
		return nil
	case "value", "raw":
		if len(args) != 1 {
			return ctx.errf("function %q requires exactly one string varbind name", name)
		}
		arg, ok := args[0].(*parse.StringNode)
		if !ok {
			return ctx.errf("function %q requires a string varbind name", name)
		}
		vb, ok := templateVarbindDef(ctx.td, ctx.fileVarbinds, arg.Text)
		if !ok {
			return ctx.errf("function %q references unknown varbind %q", name, arg.Text)
		}
		if ctx.isLabel() && !isBoundedLabelVarbind(vb) {
			return ctx.errf("label %q references unbounded varbind %q", ctx.labelKey, arg.Text)
		}
		return nil
	case "first":
		if len(args) == 0 {
			return ctx.errf("function %q requires at least one argument", name)
		}
		for _, arg := range args {
			if err := validateProfileTemplateArg(arg, ctx); err != nil {
				return err
			}
		}
		return nil
	default:
		return ctx.errf("unknown function %q", name)
	}
}

func validateProfileTemplateArg(arg parse.Node, ctx templateValidationContext) error {
	switch node := arg.(type) {
	case *parse.StringNode:
		return nil
	case *parse.DotNode:
		return nil
	case *parse.PipeNode:
		if node == nil {
			return ctx.errf("empty pipeline")
		}
		return validateProfileTemplatePipe(node, ctx)
	default:
		return ctx.errf("forbidden function argument %T", arg)
	}
}

func templateVarbindDef(td *TrapDef, fileVarbinds map[string]VarbindDef, name string) (VarbindDef, bool) {
	if name == "" {
		return VarbindDef{}, false
	}
	if vb, ok := fileVarbinds[name]; ok && vb.OID != "" {
		return vb, true
	}
	if td != nil {
		if inline := td.inlineVarbindByName(name); inline != nil && inline.OID != "" {
			return *inline, true
		}
	}
	return VarbindDef{}, false
}

func renderGoProfileTemplate(tpl *template.Template, tmpl string, entry *TrapEntry, td *TrapDef) string {
	var err error
	if tpl == nil {
		tpl, err = template.New("runtime").Funcs(runtimeTemplateFuncMap(entry, td)).Parse(tmpl)
		if err != nil {
			return fmt.Sprintf("<unresolved:template:%s>", err.Error())
		}
	} else {
		tpl, err = tpl.Clone()
		if err != nil {
			return fmt.Sprintf("<unresolved:template:%s>", err.Error())
		}
		tpl.Funcs(runtimeTemplateFuncMap(entry, td))
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, ""); err != nil {
		return fmt.Sprintf("<unresolved:template:%s>", err.Error())
	}
	return buf.String()
}

func runtimeTemplateFuncMap(entry *TrapEntry, td *TrapDef) template.FuncMap {
	return template.FuncMap{
		"hostname":       func() string { return resolveSpecialVar("_HOSTNAME", entry) },
		"source_ip":      func() string { return resolveSpecialVar("TRAP_SOURCE_IP", entry) },
		"trap_name":      func() string { return resolveSpecialVar("TRAP_NAME", entry) },
		"vendor":         func() string { return resolveSpecialVar("TRAP_DEVICE_VENDOR", entry) },
		"trap_interface": func() string { return resolveSpecialVar("TRAP_INTERFACE", entry) },
		"trap_neighbors": func() string { return resolveSpecialVar("TRAP_NEIGHBORS", entry) },
		"value":          func(name string) string { return resolveTemplateVarbind(name, entry, td, false) },
		"raw":            func(name string) string { return resolveTemplateVarbind(name, entry, td, true) },
		"first":          firstNonEmptyTemplateValue,
	}
}

func firstNonEmptyTemplateValue(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func resolveTemplateVarbind(name string, entry *TrapEntry, td *TrapDef, raw bool) string {
	if td == nil || name == "" {
		return ""
	}
	vb := td.varbindByName(name)
	if vb == nil {
		return ""
	}
	v, ok := findVarbindForProfileOID(entry, vb.OID)
	if !ok {
		return ""
	}
	if raw {
		return varbindRawValue(v)
	}
	return varbindDisplayValue(v, vb)
}
