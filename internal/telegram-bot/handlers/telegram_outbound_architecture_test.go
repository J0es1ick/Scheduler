package handlers

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHandlersCannotBypassTelegramOutboundPolicy(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test location")
	}
	directory := filepath.Dir(currentFile)
	files, err := filepath.Glob(filepath.Join(directory, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	blocked := map[string]struct{}{
		"Send": {}, "SendAlbum": {}, "Reply": {}, "Forward": {}, "ForwardTo": {},
		"Edit": {}, "EditCaption": {}, "Delete": {}, "Notify": {}, "Answer": {}, "Respond": {},
	}
	for _, file := range files {
		if filepath.Base(file) == "telegram_outbound.go" {
			continue
		}
		parsed, parseErr := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", file, parseErr)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			call, isCall := node.(*ast.CallExpr)
			if !isCall {
				return true
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector {
				return true
			}
			if _, isBlocked := blocked[selector.Sel.Name]; !isBlocked {
				return true
			}
			botCall, isBotCall := selector.X.(*ast.CallExpr)
			if !isBotCall {
				return true
			}
			botSelector, isBotSelector := botCall.Fun.(*ast.SelectorExpr)
			if isBotSelector && botSelector.Sel.Name == "Bot" {
				t.Errorf("%s calls Bot().%s outside Telegram outbound gateway", filepath.Base(file), selector.Sel.Name)
			}
			return true
		})
	}
}
