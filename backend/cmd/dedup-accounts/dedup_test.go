package main

import (
	"testing"
	"time"
)

func TestPlanDedup(t *testing.T) {
	now := time.Now()
	rows := []accountRow{
		{ID: 41, UUID: "A", Name: "a2", CreatedAt: now.Add(-1 * time.Hour)},
		{ID: 1, UUID: "A", Name: "a1", CreatedAt: now.Add(-2 * time.Hour)}, // earliest -> keep
		{ID: 2, UUID: "B", Name: "b1", CreatedAt: now},                     // solo, ignored
	}
	plan := planDedup(rows)
	if len(plan.Groups) != 1 {
		t.Fatalf("want 1 duplicate group, got %d", len(plan.Groups))
	}
	g := plan.Groups[0]
	if g.KeepID != 1 {
		t.Errorf("keep earliest-created id=1, got keep=%d", g.KeepID)
	}
	if len(g.RemoveIDs) != 1 || g.RemoveIDs[0] != 41 {
		t.Errorf("remove=[41] expected, got %v", g.RemoveIDs)
	}
}

func TestPlanDedup_TieBreakByID(t *testing.T) {
	ts := time.Now()
	rows := []accountRow{
		{ID: 41, UUID: "A", CreatedAt: ts},
		{ID: 1, UUID: "A", CreatedAt: ts}, // same created -> keep lowest id
	}
	plan := planDedup(rows)
	if len(plan.Groups) != 1 || plan.Groups[0].KeepID != 1 {
		t.Fatalf("same created should keep lowest id=1, got %+v", plan.Groups)
	}
}

func TestPlanDedup_NoDup(t *testing.T) {
	rows := []accountRow{
		{ID: 1, UUID: "A", CreatedAt: time.Now()},
		{ID: 2, UUID: "B", CreatedAt: time.Now()},
		{ID: 3, UUID: "", CreatedAt: time.Now()}, // no uuid ignored
	}
	if plan := planDedup(rows); len(plan.Groups) != 0 {
		t.Errorf("no duplicates expected, got %+v", plan.Groups)
	}
}
