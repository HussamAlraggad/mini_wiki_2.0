package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestStore creates a MemStore backed by a temp directory.
func newTestStore(t *testing.T) MemStore {
	t.Helper()
	dir, err := os.MkdirTemp("", "memory_test")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}

	// Override HOME to use temp dir for Init()
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	t.Cleanup(func() {
		os.Setenv("HOME", origHome)
		os.RemoveAll(dir)
	})

	m := New()
	if err := m.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}
	return m
}

func TestInit(t *testing.T) {
	dir, err := os.MkdirTemp("", "memory_test_init")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.RemoveAll(dir)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", origHome)

	m := New()
	if err := m.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Verify directory exists
	expected := filepath.Join(dir, ".config", "mini-wiki", "memory")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("Init should create %s: %v", expected, err)
	}
}

func TestRegisterSkill(t *testing.T) {
	m := newTestStore(t)

	skill := Skill{
		Name:        "TestSkill",
		Description: "A test skill",
		Command:     "/test",
		Category:    "testing",
		Models:      []string{"model1"},
	}
	if err := m.RegisterSkill(skill); err != nil {
		t.Fatalf("RegisterSkill failed: %v", err)
	}
}

func TestRegisterSkillUpdate(t *testing.T) {
	m := newTestStore(t)

	m.RegisterSkill(Skill{Name: "Skill1", Description: "v1", Category: "cat1"})
	m.RegisterSkill(Skill{Name: "Skill1", Description: "v2", Category: "cat1"})

	skills, _ := m.GetSkills("")
	if len(skills) != 1 {
		t.Errorf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Description != "v2" {
		t.Errorf("description = %q, want %q", skills[0].Description, "v2")
	}
}

func TestGetSkills(t *testing.T) {
	m := newTestStore(t)

	m.RegisterSkill(Skill{Name: "S1", Description: "desc1", Category: "catA"})
	m.RegisterSkill(Skill{Name: "S2", Description: "desc2", Category: "catB"})
	m.RegisterSkill(Skill{Name: "S3", Description: "desc3", Category: "catA"})

	all, err := m.GetSkills("")
	if err != nil {
		t.Fatalf("GetSkills failed: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("GetSkills('') = %d, want 3", len(all))
	}

	catA, _ := m.GetSkills("catA")
	if len(catA) != 2 {
		t.Errorf("GetSkills('catA') = %d, want 2", len(catA))
	}

	catB, _ := m.GetSkills("catB")
	if len(catB) != 1 {
		t.Errorf("GetSkills('catB') = %d, want 1", len(catB))
	}

	catNone, _ := m.GetSkills("nonexistent")
	if len(catNone) != 0 {
		t.Errorf("GetSkills('nonexistent') = %d, want 0", len(catNone))
	}
}

func TestGetSkill(t *testing.T) {
	m := newTestStore(t)

	m.RegisterSkill(Skill{Name: "MySkill", Description: "found me"})

	skill, err := m.GetSkill("MySkill")
	if err != nil {
		t.Fatalf("GetSkill failed: %v", err)
	}
	if skill == nil {
		t.Fatal("GetSkill returned nil")
	}
	if skill.Name != "MySkill" {
		t.Errorf("Name = %q, want %q", skill.Name, "MySkill")
	}
}

func TestGetSkillNotFound(t *testing.T) {
	m := newTestStore(t)

	skill, err := m.GetSkill("nonexistent")
	if err != nil {
		t.Fatalf("GetSkill failed: %v", err)
	}
	if skill != nil {
		t.Errorf("expected nil for nonexistent skill, got %+v", skill)
	}
}

func TestListCategories(t *testing.T) {
	m := newTestStore(t)

	m.RegisterSkill(Skill{Name: "S1", Category: "data"})
	m.RegisterSkill(Skill{Name: "S2", Category: "export"})
	m.RegisterSkill(Skill{Name: "S3", Category: "data"})

	cats, err := m.ListCategories()
	if err != nil {
		t.Fatalf("ListCategories failed: %v", err)
	}
	if len(cats) != 2 {
		t.Errorf("expected 2 categories, got %d: %v", len(cats), cats)
	}
}

func TestListCategoriesEmpty(t *testing.T) {
	m := newTestStore(t)

	cats, err := m.ListCategories()
	if err != nil {
		t.Fatalf("ListCategories failed: %v", err)
	}
	if len(cats) != 0 {
		t.Errorf("expected 0 categories, got %d", len(cats))
	}
}

func TestLogFlaw(t *testing.T) {
	m := newTestStore(t)

	flaw := FlawEntry{
		Title:       "Test flaw",
		Description: "A flaw for testing",
		Symptom:     "nothing works",
		Cause:       "test bug",
		Solution:    "fix it",
	}
	if err := m.LogFlaw(flaw); err != nil {
		t.Fatalf("LogFlaw failed: %v", err)
	}
}

func TestLogFlawAutoID(t *testing.T) {
	m := newTestStore(t)

	m.LogFlaw(FlawEntry{Title: "Flaw1"})
	m.LogFlaw(FlawEntry{Title: "Flaw2"})

	flaws, _ := m.GetFlaws(nil)
	if len(flaws) != 2 {
		t.Fatalf("expected 2 flaws, got %d", len(flaws))
	}
	// Auto-generated IDs should be "FLAW-001" and "FLAW-002"
	if !strings.HasPrefix(flaws[0].ID, "FLAW-") {
		t.Errorf("expected auto-generated ID starting with FLAW-, got %q", flaws[0].ID)
	}
	// IDs should be different
	if flaws[0].ID == flaws[1].ID {
		t.Errorf("two flaws should have different IDs, both got %q", flaws[0].ID)
	}
}

func TestLogFlawWithCustomID(t *testing.T) {
	m := newTestStore(t)

	flaw := FlawEntry{ID: "CUSTOM-001", Title: "Custom ID flaw"}
	m.LogFlaw(flaw)

	got, err := m.GetFlaw("CUSTOM-001")
	if err != nil {
		t.Fatalf("GetFlaw failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetFlaw returned nil")
	}
	if got.Title != "Custom ID flaw" {
		t.Errorf("Title = %q, want %q", got.Title, "Custom ID flaw")
	}
}

func TestGetFlaws(t *testing.T) {
	m := newTestStore(t)

	m.LogFlaw(FlawEntry{Title: "OpenFlaw", Resolved: false})
	m.LogFlaw(FlawEntry{Title: "ResolvedFlaw", Resolved: true})
	m.LogFlaw(FlawEntry{Title: "AnotherOpen", Resolved: false})

	// All flaws
	all, _ := m.GetFlaws(nil)
	if len(all) != 3 {
		t.Errorf("GetFlaws(nil) = %d, want 3", len(all))
	}

	// Only resolved
	resTrue := true
	resolved, _ := m.GetFlaws(&resTrue)
	if len(resolved) != 1 {
		t.Errorf("GetFlaws(resolved=true) = %d, want 1", len(resolved))
	}

	// Only unresolved
	resFalse := false
	unresolved, _ := m.GetFlaws(&resFalse)
	if len(unresolved) != 2 {
		t.Errorf("GetFlaws(resolved=false) = %d, want 2", len(unresolved))
	}
}

func TestGetFlaw(t *testing.T) {
	m := newTestStore(t)

	m.LogFlaw(FlawEntry{Title: "FindMe", Description: "found it"})
	flaws, _ := m.GetFlaws(nil)

	got, err := m.GetFlaw(flaws[0].ID)
	if err != nil {
		t.Fatalf("GetFlaw failed: %v", err)
	}
	if got == nil {
		t.Fatal("GetFlaw returned nil")
	}
	if got.Title != "FindMe" {
		t.Errorf("Title = %q, want %q", got.Title, "FindMe")
	}
}

func TestGetFlawNotFound(t *testing.T) {
	m := newTestStore(t)

	got, err := m.GetFlaw("NONEXISTENT")
	if err != nil {
		t.Fatalf("GetFlaw failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for nonexistent flaw, got %+v", got)
	}
}

func TestResolveFlaw(t *testing.T) {
	m := newTestStore(t)

	m.LogFlaw(FlawEntry{Title: "FixMe", Resolved: false})
	flaws, _ := m.GetFlaws(nil)
	id := flaws[0].ID

	if err := m.ResolveFlaw(id); err != nil {
		t.Fatalf("ResolveFlaw failed: %v", err)
	}

	got, _ := m.GetFlaw(id)
	if got == nil {
		t.Fatal("GetFlaw after resolve returned nil")
	}
	if !got.Resolved {
		t.Errorf("flaw should be resolved after ResolveFlaw")
	}
	if got.ResolvedAt.IsZero() {
		t.Errorf("ResolvedAt should be set after resolve")
	}
}

func TestResolveFlawNotFound(t *testing.T) {
	m := newTestStore(t)

	err := m.ResolveFlaw("NONEXISTENT")
	if err == nil {
		t.Fatal("expected error for resolving nonexistent flaw")
	}
}

func TestSaveLoadSession(t *testing.T) {
	m := newTestStore(t)

	session := SessionState{
		LastProject: "/home/test/project",
		LastQuery:   "find relevant rows",
		ActiveModel: "gemma4:e4b",
	}

	if err := m.SaveSession(session); err != nil {
		t.Fatalf("SaveSession failed: %v", err)
	}

	loaded, err := m.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession failed: %v", err)
	}
	if loaded == nil {
		t.Fatal("LoadSession returned nil")
	}
	if loaded.LastProject != session.LastProject {
		t.Errorf("LastProject = %q, want %q", loaded.LastProject, session.LastProject)
	}
	if loaded.ActiveModel != session.ActiveModel {
		t.Errorf("ActiveModel = %q, want %q", loaded.ActiveModel, session.ActiveModel)
	}
}

func TestLoadSessionEmpty(t *testing.T) {
	// A freshly initialized store should have no session
	dir, err := os.MkdirTemp("", "memory_test_empty")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	defer os.RemoveAll(dir)

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", dir)
	defer os.Setenv("HOME", origHome)

	m := New()
	m.Init()

	session, err := m.LoadSession()
	if err != nil {
		t.Fatalf("LoadSession on empty store: %v", err)
	}
	if session == nil {
		t.Fatal("LoadSession returned nil")
	}
	// Empty session should be zero-valued, not an error
	if session.LastProject != "" {
		t.Errorf("expected empty session, got %+v", session)
	}
}

func TestMemStoreInterface(t *testing.T) {
	var m MemStore = New()
	if m == nil {
		t.Fatal("New() returned nil")
	}
}

func TestFlawTimestamps(t *testing.T) {
	m := newTestStore(t)

	m.LogFlaw(FlawEntry{Title: "Timestamp test", Resolved: false})
	flaws, _ := m.GetFlaws(nil)
	if len(flaws) == 0 {
		t.Fatal("no flaws found")
	}
	if flaws[0].CreatedAt.IsZero() {
		t.Errorf("CreatedAt should be set automatically")
	}
}
