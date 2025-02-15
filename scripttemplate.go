package templ

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
)

// ComponentScript is a templ Script template.
type ComponentScript struct {
	// Name of the script, e.g. print.
	Name string
	// Function to render.
	Function string
	// Call of the function in JavaScript syntax, including parameters, and
	// ensures parameters are HTML escaped; useful for injecting into HTML
	// attributes like onclick, onhover, etc.
	//
	// Given:
	//    functionName("some string",12345)
	// It would render:
	//    __templ_functionName_sha(&#34;some string&#34;,12345))
	//
	// This is can be injected into HTML attributes:
	//    <button onClick="__templ_functionName_sha(&#34;some string&#34;,12345))">Click Me</button>
	Call string
	// Call of the function in JavaScript syntax, including parameters. It
	// does not HTML escape parameters; useful for directly calling in script
	// elements.
	//
	// Given:
	//    functionName("some string",12345)
	// It would render:
	//    __templ_functionName_sha("some string",12345))
	//
	// This is can be used to call the function inside a script tag:
	//    <script>__templ_functionName_sha("some string",12345))</script>
	CallInline string
}

var _ Component = ComponentScript{}

func writeScriptHeader(ctx context.Context, w io.Writer) (err error) {
	var nonceAttr string
	if nonce := GetNonce(ctx); nonce != "" {
		nonceAttr = " nonce=\"" + EscapeString(nonce) + "\""
	}
	_, err = fmt.Fprintf(w, `<script%s>`, nonceAttr)
	return err
}

func (c ComponentScript) Render(ctx context.Context, w io.Writer) error {
	err := RenderScriptItems(ctx, w, c)
	if err != nil {
		return err
	}
	if len(c.Call) > 0 {
		if err = writeScriptHeader(ctx, w); err != nil {
			return err
		}
		if _, err = io.WriteString(w, c.CallInline); err != nil {
			return err
		}
		if _, err = io.WriteString(w, `</script>`); err != nil {
			return err
		}
	}
	return nil
}

// RenderScriptItems renders a <script> element, if the script has not already been rendered.
func RenderScriptItems(ctx context.Context, w io.Writer, scripts ...ComponentScript) (err error) {
	if len(scripts) == 0 {
		return nil
	}
	_, v := getContext(ctx)
	sb := new(strings.Builder)
	for _, s := range scripts {
		if !v.hasScriptBeenRendered(s.Name) {
			sb.WriteString(s.Function)
			v.addScript(s.Name)
		}
	}
	if sb.Len() > 0 {
		if err = writeScriptHeader(ctx, w); err != nil {
			return err
		}
		if _, err = io.WriteString(w, sb.String()); err != nil {
			return err
		}
		if _, err = io.WriteString(w, `</script>`); err != nil {
			return err
		}
	}
	return nil
}

// JSExpression represents a JavaScript expression intended for use as an argument for script templates.
// The string value of JSExpression will be inserted directly as JavaScript code in function call arguments.
type JSExpression string

// SafeScript encodes unknown parameters for safety for inside HTML attributes.
func SafeScript(functionName string, params ...any) string {
	if !jsFunctionName.MatchString(functionName) {
		functionName = "__templ_invalid_js_function_name"
	}
	sb := new(strings.Builder)
	sb.WriteString(html.EscapeString(functionName))
	sb.WriteRune('(')
	for i, p := range params {
		sb.WriteString(EscapeString(jsonEncodeParam(p)))
		if i < len(params)-1 {
			sb.WriteRune(',')
		}
	}
	sb.WriteRune(')')
	return sb.String()
}

// SafeScript encodes unknown parameters for safety for inline scripts.
func SafeScriptInline(functionName string, params ...any) string {
	if !jsFunctionName.MatchString(functionName) {
		functionName = "__templ_invalid_js_function_name"
	}
	sb := new(strings.Builder)
	sb.WriteString(functionName)
	sb.WriteRune('(')
	for i, p := range params {
		sb.WriteString(jsonEncodeParam(p))
		if i < len(params)-1 {
			sb.WriteRune(',')
		}
	}
	sb.WriteRune(')')
	return sb.String()
}

func jsonEncodeParam(param any) string {
	if val, ok := param.(JSExpression); ok {
		return string(val)
	}
	enc, _ := json.Marshal(param)
	return string(enc)
}

// isValidJSFunctionName returns true if the given string is a valid JavaScript function name, e.g. console.log, alert, etc.
var jsFunctionName = regexp.MustCompile(`^([$_a-zA-Z][$_a-zA-Z0-9]+\.?)+$`)
