package zfs

import (
	"sync"
)

var (
	onceBookmarks sync.Once
	onceMultiSnap sync.Once

	haveBookmarks bool
	haveMultiSnap bool
)

// HasBookmarks checks for support of 'feature@bookmarks'
func HasBookmarks(debug bool) bool {
	onceBookmarks.Do(func() {
		pools, err := ListPools("", []string{"feature@bookmarks"}, debug)
		if err != nil {
			haveBookmarks = false

			return
		}

		for _, pool := range pools {
			if _, ok := pool.Properties["feature@bookmarks"]; ok {
				haveBookmarks = true

				return
			}
		}

		haveBookmarks = false
	})

	return haveBookmarks
}

// HasMultiSnap piggybacks on HasBookmarks
func HasMultiSnap(debug bool) bool {
	onceMultiSnap.Do(func() {
		haveMultiSnap = HasBookmarks(debug)
	})

	return haveMultiSnap
}
