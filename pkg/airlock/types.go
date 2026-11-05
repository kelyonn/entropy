package airlock

type ExecRequest struct {
	SessionID   string `json:"sessionId"`
	ContainerID string `json:"containerId"`
	Command     string `json:"command"`
}

type ExecChunk struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	Completed bool   `json:"completed"`
	ExitCode  int    `json:"exitCode,omitempty"`
}

type SpawnRequest struct {
	SessionID           string `json:"sessionId"`
	ExistingContainerID string `json:"existingContainerId,omitempty"`
	Image               string `json:"image,omitempty"`
}

type CommitRequest struct {
	ContainerID string `json:"containerId"`
	Tag         string `json:"tag"`
}

type CommitResponse struct {
	ImageID string `json:"imageId"`
	Tag     string `json:"tag"`
}

type SaveImageRequest struct {
	Tag string `json:"tag"`
}

type LoadImageRequest struct {
	Data []byte `json:"data"`
}

type SpawnResponse struct {
	ContainerID string `json:"containerId"`
}

type KillRequest struct {
	ContainerID string `json:"containerId"`
	ExecID      string `json:"execId,omitempty"`
	Signal      string `json:"signal"` // SIGINT, SIGTERM, SIGKILL, SIGHUP, or SIGQUIT
}

type StatsRequest struct {
	ContainerID string `json:"containerId"`
}

type StatsResponse struct {
	CPU         int    `json:"cpu"`
	Memory      int    `json:"memory"`
	Pids        int    `json:"pids"`
	MemoryBytes uint64 `json:"memoryBytes,omitempty"`
	MemoryLimit uint64 `json:"memoryLimit,omitempty"`
}

type StopRequest struct {
	SessionID   string `json:"sessionId"`
	ContainerID string `json:"containerId"`
	GraceSec    int    `json:"graceSec"`
}
