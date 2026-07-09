package service

import (
	"strings"
	"testing"
)

// 兜底关键词必须覆盖用户原话里点名的三大类：生物武器 / 逆向工程 / 色情。
// 任何一类缺失都是单点封号风险。
func TestDefaultBlockedKeywordsBioAndReverse_CoversAllCategories(t *testing.T) {
	all := strings.ToLower(strings.Join(defaultBlockedKeywordsBioAndReverse, " "))

	categories := []struct {
		name     string
		needAny  []string
	}{
		{"生物武器", []string{"bioweapon", "生物武器", "anthrax", "ricin"}},
		{"逆向工程", []string{"reverse engineer", "decompile", "逆向", "0day"}},
		{"色情/未成年", []string{"csam", "minor sexual", "色情", "幼女"}},
	}
	for _, c := range categories {
		found := false
		for _, w := range c.needAny {
			if strings.Contains(all, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("兜底关键词缺 %q 类目，需至少匹配 %v", c.name, c.needAny)
		}
	}
}

func TestMergeBlockedKeywords_DefaultsFirst(t *testing.T) {
	custom := []string{"  ", "MyPhrasing", "myphrasing", "extra"}
	merged := mergeBlockedKeywords(defaultBlockedKeywordsBioAndReverse, custom)
	if len(merged) < len(defaultBlockedKeywordsBioAndReverse)+1 {
		t.Fatalf("merged len=%d, want >= %d", len(merged), len(defaultBlockedKeywordsBioAndReverse)+1)
	}
	for i := 0; i < len(defaultBlockedKeywordsBioAndReverse); i++ {
		if merged[i] != defaultBlockedKeywordsBioAndReverse[i] {
			t.Errorf("defaults must come first; merged[%d]=%q", i, merged[i])
		}
	}
	hasExtra := false
	hasDedup := false // we keep at most ONE of the two case variants
	for _, k := range merged[len(defaultBlockedKeywordsBioAndReverse):] {
		if k == "extra" {
			hasExtra = true
		}
		if k == "MyPhrasing" || k == "myphrasing" {
			hasDedup = true
		}
	}
	if !hasExtra {
		t.Errorf("merged misses the truly new custom kw")
	}
	if !hasDedup {
		t.Errorf("merged lost the (single surviving) custom case variant")
	}
}

func TestMergeBlockedKeywords_Empty(t *testing.T) {
	if got := mergeBlockedKeywords(nil, nil); len(got) != 0 {
		t.Errorf("nil+nil = %v, want empty", got)
	}
	if got := mergeBlockedKeywords(defaultBlockedKeywordsBioAndReverse, nil); len(got) != len(defaultBlockedKeywordsBioAndReverse) {
		t.Errorf("defaults preserved when custom nil: got %d", len(got))
	}
}