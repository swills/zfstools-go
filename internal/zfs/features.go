package zfs

import "sync"

type featureCache struct {
	mutex            sync.Mutex
	haveBookmarks    bool
	bookmarksChecked bool
}

func (client Client) hasBookmarks(debug bool) bool {
	client.featureCache.mutex.Lock()
	defer client.featureCache.mutex.Unlock()

	if !client.featureCache.bookmarksChecked {
		client.featureCache.haveBookmarks = client.checkBookmarks(debug)
		client.featureCache.bookmarksChecked = true
	}

	return client.featureCache.haveBookmarks
}

func (client Client) checkBookmarks(debug bool) bool {
	pools, err := client.ListPools("", []string{"feature@bookmarks"}, debug)
	if err != nil {
		return false
	}

	for _, pool := range pools {
		if _, ok := pool.Properties["feature@bookmarks"]; ok {
			return true
		}
	}

	return false
}
