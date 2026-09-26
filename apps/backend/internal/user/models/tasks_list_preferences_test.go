package models

import "testing"

// AC-PLUGINS-TASKLIST-FACETS-003.2 and 003.4: shape validation preserves a saved facet.
func TestTaskListFacetPreferences(t *testing.T) {
	accepted := []string{
		"facet:plugin:tags", "facet:com.example.tags:tag-name", "facet:my_plugin:tags",
		" facet:plugin:tags ",
	}
	for _, value := range accepted {
		t.Run(value, func(t *testing.T) {
			if !IsValidTasksListSort(value) || !IsValidTasksListGroup(value) {
				t.Fatalf("sort/group rejected valid facet %q", value)
			}
			want := "facet:plugin:tags"
			if value == "facet:com.example.tags:tag-name" || value == "facet:my_plugin:tags" {
				want = value
			}
			if got := NormalizeTasksListSort(value); got != want {
				t.Fatalf("sort = %q, want %q", got, want)
			}
			if got := NormalizeTasksListGroup(value); got != want {
				t.Fatalf("group = %q, want %q", got, want)
			}
		})
	}
	for _, value := range []string{"facet:", "facet:Plugin:tags", "facet:plugin:tag id", "facet:plugin:", "facet:plugin:UPPER", "facet:plugin:tags:extra", "unknown", ""} {
		t.Run("reject_"+value, func(t *testing.T) {
			if IsValidTasksListSort(value) || IsValidTasksListGroup(value) {
				t.Fatalf("sort/group accepted malformed facet %q", value)
			}
			if NormalizeTasksListSort(value) != TasksListSortDefault || NormalizeTasksListGroup(value) != TasksListGroupDefault {
				t.Fatalf("malformed facet %q did not normalize to defaults", value)
			}
		})
	}
	if !IsValidTasksListSort(" title_asc ") || !IsValidTasksListGroup(" workflow ") {
		t.Fatal("built-in preferences must remain accepted")
	}
}

// AC-PLUGINS-TASKLIST-FACETS-002.7: a page-local facet cannot enter a task-list query.
func TestTasksListApiSort(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"facet:plugin:tags", TasksListSortDefault},
		{" facet:plugin:tags ", TasksListSortDefault},
		{"title_asc", TasksListSortTitleAsc},
		{" title_asc ", TasksListSortTitleAsc},
		{"invalid", TasksListSortDefault},
	} {
		if got := TasksListApiSort(tc.input); got != tc.want {
			t.Errorf("TasksListApiSort(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
