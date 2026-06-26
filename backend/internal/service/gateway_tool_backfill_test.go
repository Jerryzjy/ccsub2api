package service

import (
	"testing"

	"github.com/tidwall/gjson"
)

// backfillDanglingToolDeclarations 兜底修复客户端发来的"tool_use/tool_choice 引用了
// 未在 tools[] 声明的工具"不一致，避免上游
// "Tool reference 'X' not found in available tools" 400。
// 对悬空工具名补一个最小 schema 的占位声明，使请求自洽。

// 悬空 tool_use 引用 → 补进 tools[]，消除悬空。
func TestBackfillDanglingToolDeclarations_AddsMissingTool(t *testing.T) {
	body := []byte(`{"tools":[{"name":"Bash","input_schema":{"type":"object"}}],"messages":[
		{"role":"assistant","content":[
			{"type":"tool_use","id":"tu_1","name":"TaskCreate","input":{}}
		]}
	]}`)
	got := backfillDanglingToolDeclarations(body)

	if d := findDanglingToolRefs(got); len(d) != 0 {
		t.Errorf("expected no dangling after backfill, got %v", d)
	}
	// TaskCreate 应已被声明，且带合法 input_schema。
	found := false
	gjson.GetBytes(got, "tools").ForEach(func(_, tool gjson.Result) bool {
		if tool.Get("name").String() == "TaskCreate" {
			found = true
			if tool.Get("input_schema.type").String() != "object" {
				t.Errorf("backfilled tool must have object input_schema, got %s", tool.Raw)
			}
		}
		return true
	})
	if !found {
		t.Errorf("TaskCreate not backfilled into tools: %s", gjson.GetBytes(got, "tools").Raw)
	}
}

// 无悬空时不改动 body。
func TestBackfillDanglingToolDeclarations_NoopWhenConsistent(t *testing.T) {
	body := []byte(`{"tools":[{"name":"Bash","input_schema":{"type":"object"}}],"messages":[
		{"role":"assistant","content":[
			{"type":"tool_use","id":"tu_1","name":"Bash","input":{}}
		]}
	]}`)
	got := backfillDanglingToolDeclarations(body)
	if string(got) != string(body) {
		t.Errorf("consistent body must be unchanged.\n got: %s\nwant: %s", got, body)
	}
}

// tool_choice 悬空引用也应被补声明。
func TestBackfillDanglingToolDeclarations_AddsToolChoiceRef(t *testing.T) {
	body := []byte(`{"tools":[{"name":"Bash","input_schema":{"type":"object"}}],"tool_choice":{"type":"tool","name":"Grep"}}`)
	got := backfillDanglingToolDeclarations(body)
	if d := findDanglingToolRefs(got); len(d) != 0 {
		t.Errorf("expected no dangling after backfill, got %v", d)
	}
}

// 没有 tools 字段（但有悬空 tool_use）时也要建出 tools 数组。
func TestBackfillDanglingToolDeclarations_CreatesToolsArray(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"assistant","content":[
			{"type":"tool_use","id":"tu_1","name":"TaskCreate","input":{}}
		]}
	]}`)
	got := backfillDanglingToolDeclarations(body)
	if d := findDanglingToolRefs(got); len(d) != 0 {
		t.Errorf("expected no dangling after backfill, got %v", d)
	}
	if !gjson.GetBytes(got, "tools").IsArray() {
		t.Errorf("tools array must be created, got: %s", got)
	}
}
