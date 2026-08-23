package zfs

import "testing"

func TestFeatureDetectorHasBookmarks(t *testing.T) {
	t.Parallel()

	tests := []struct {
		listPools func(string, []string, bool) ([]Pool, error)
		name      string
		want      bool
	}{
		{
			name: "supported",
			listPools: func(_ string, _ []string, _ bool) ([]Pool, error) {
				return []Pool{{Properties: map[string]string{"feature@bookmarks": "enabled"}}}, nil
			},
			want: true,
		},
		{
			name: "unsupported",
			listPools: func(_ string, _ []string, _ bool) ([]Pool, error) {
				return []Pool{{Properties: map[string]string{}}}, nil
			},
			want: false,
		},
		{
			name: "error",
			listPools: func(_ string, _ []string, _ bool) ([]Pool, error) {
				return nil, errTestCommand
			},
			want: false,
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			detector := &featureDetector{}
			if got := detector.hasBookmarks(testCase.listPools, false); got != testCase.want {
				t.Errorf("hasBookmarks() = %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestFeatureDetectorHasMultiSnap(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name string
		pool Pool
		want bool
	}{
		{name: "supported", pool: Pool{Properties: map[string]string{"feature@bookmarks": "enabled"}}, want: true},
		{name: "unsupported", pool: Pool{Properties: map[string]string{}}, want: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			detector := &featureDetector{}
			listPools := func(_ string, _ []string, _ bool) ([]Pool, error) {
				return []Pool{testCase.pool}, nil
			}

			if got := detector.hasMultiSnap(listPools, false); got != testCase.want {
				t.Errorf("hasMultiSnap() = %v, want %v", got, testCase.want)
			}
		})
	}
}
