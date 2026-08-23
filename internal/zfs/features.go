package zfs

import "sync"

type featureDetector struct {
	onceBookmarks sync.Once
	onceMultiSnap sync.Once
	haveBookmarks bool
	haveMultiSnap bool
}

func (detector *featureDetector) hasBookmarks(
	listPools func(string, []string, bool) ([]Pool, error),
	debug bool,
) bool {
	detector.onceBookmarks.Do(func() {
		pools, err := listPools("", []string{"feature@bookmarks"}, debug)
		if err != nil {
			detector.haveBookmarks = false

			return
		}

		for _, pool := range pools {
			if _, ok := pool.Properties["feature@bookmarks"]; ok {
				detector.haveBookmarks = true

				return
			}
		}

		detector.haveBookmarks = false
	})

	return detector.haveBookmarks
}

func (detector *featureDetector) hasMultiSnap(
	listPools func(string, []string, bool) ([]Pool, error),
	debug bool,
) bool {
	detector.onceMultiSnap.Do(func() {
		detector.haveMultiSnap = detector.hasBookmarks(listPools, debug)
	})

	return detector.haveMultiSnap
}
