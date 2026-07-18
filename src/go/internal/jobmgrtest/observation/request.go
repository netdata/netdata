package observation

type OfferedRequest struct {
	Sequence         int
	Class            string
	Key              string
	UID              string
	RequestSHA256    string
	FollowupSHA256   string
	UsefulWorkSHA256 string
	OfferedMonoNS    int64
	FollowupMonoNS   int64
}
