package models

import (
	"regexp"
	"slices"
	"strings"
)

const (
	TasksListSortUpdatedDesc = "updated_desc"
	TasksListSortUpdatedAsc  = "updated_asc"
	TasksListSortCreatedDesc = "created_desc"
	TasksListSortCreatedAsc  = "created_asc"
	TasksListSortTitleAsc    = "title_asc"
	TasksListSortTitleDesc   = "title_desc"
	TasksListSortDefault     = TasksListSortUpdatedDesc

	TasksListGroupState      = "state"
	TasksListGroupWorkflow   = "workflow"
	TasksListGroupRepository = "repository"
	TasksListGroupNone       = "none"
	TasksListGroupDefault    = TasksListGroupState
)

const taskListFacetPrefix = "facet:"

var (
	taskListPluginIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	taskListFacetIDPattern  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

var (
	tasksListSortValues = []string{
		TasksListSortUpdatedDesc,
		TasksListSortUpdatedAsc,
		TasksListSortCreatedDesc,
		TasksListSortCreatedAsc,
		TasksListSortTitleAsc,
		TasksListSortTitleDesc,
	}
	tasksListGroupValues = []string{
		TasksListGroupState,
		TasksListGroupWorkflow,
		TasksListGroupRepository,
		TasksListGroupNone,
	}
)

func TasksListSortValues() []string {
	return append([]string(nil), tasksListSortValues...)
}

func TasksListGroupValues() []string {
	return append([]string(nil), tasksListGroupValues...)
}

// IsTaskListFacetValue accepts an encoded selection by shape, not by installed plugin.
func IsTaskListFacetValue(value string) bool {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, taskListFacetPrefix) {
		return false
	}
	pluginID, facetID, found := strings.Cut(strings.TrimPrefix(value, taskListFacetPrefix), ":")
	return found && taskListPluginIDPattern.MatchString(pluginID) && taskListFacetIDPattern.MatchString(facetID)
}

func IsValidTasksListSort(value string) bool {
	return slices.Contains(tasksListSortValues, strings.TrimSpace(value)) || IsTaskListFacetValue(value)
}

func IsValidTasksListGroup(value string) bool {
	return slices.Contains(tasksListGroupValues, strings.TrimSpace(value)) || IsTaskListFacetValue(value)
}

func NormalizeTasksListSort(value string) string {
	value = strings.TrimSpace(value)
	if IsValidTasksListSort(value) {
		return value
	}
	return TasksListSortDefault
}

// TasksListApiSort resolves a requested facet to a built-in database ordering.
func TasksListApiSort(value string) string {
	if IsTaskListFacetValue(value) {
		return TasksListSortDefault
	}
	return NormalizeTasksListSort(value)
}

func NormalizeTasksListGroup(value string) string {
	value = strings.TrimSpace(value)
	if IsValidTasksListGroup(value) {
		return value
	}
	return TasksListGroupDefault
}
