package provider

type Change struct {
	Path         string
	PreviousPath string
	Patch        string
	Sha          string
	Additions    int
	Deletions    int
	Changes      int
	Added        bool
	Renamed      bool
	Deleted      bool
}
