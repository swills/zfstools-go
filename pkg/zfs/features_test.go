package zfs

import (
	"sync"
	"testing"
)

func TestHasBookmarks_True(t *testing.T) {
	resetFeatures()

	listPoolsFn = func(_ string, _ []string, _ bool) ([]Pool, error) {
		return []Pool{
			{Properties: map[string]string{"feature@bookmarks": "enabled"}},
		}, nil
	}

	if !HasBookmarks(false) {
		t.Fatal("expected HasBookmarks to return true")
	}
}

func TestHasBookmarks_False(t *testing.T) {
	resetFeatures()

	listPoolsFn = func(_ string, _ []string, _ bool) ([]Pool, error) {
		return []Pool{
			{Properties: map[string]string{}},
		}, nil
	}

	if HasBookmarks(false) {
		t.Fatal("expected HasBookmarks to return false")
	}
}

func TestHasBookmarks_Error(t *testing.T) {
	resetFeatures()

	listPoolsFn = func(_ string, _ []string, _ bool) ([]Pool, error) {
		return nil, assertError("simulated failure")
	}

	if HasBookmarks(false) {
		t.Fatal("expected HasBookmarks to return false on error")
	}
}

func resetFeatures() {
	haveBookmarks = false
	haveMultiSnap = false
	listPoolsFn = ListPools
	onceBookmarks = sync.Once{}
	onceMultiSnap = sync.Once{}
}

type assertError string

func (e assertError) Error() string { return string(e) }
