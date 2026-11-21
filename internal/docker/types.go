package docker

// 本文件定义 Docker Engine API 数据结构（按需裁剪的子集）。
// 字段命名与 JSON 标签严格对齐 Docker Engine API，保证与不同版本守护进程的兼容性。

// Port 容器端口映射信息。
type Port struct {
	IP          string `json:"IP,omitempty"`
	PrivatePort int    `json:"PrivatePort"`
	PublicPort  int    `json:"PublicPort,omitempty"`
	Type        string `json:"Type"`
}

// EndpointIPAMConfig 静态 IP 配置。
type EndpointIPAMConfig struct {
	IPv4Address  string   `json:"IPv4Address,omitempty"`
	IPv6Address  string   `json:"IPv6Address,omitempty"`
	LinkLocalIPs []string `json:"LinkLocalIPs,omitempty"`
}

// EndpointSettings 容器网络端点配置。
type EndpointSettings struct {
	IPAMConfig          *EndpointIPAMConfig   `json:"IPAMConfig,omitempty"`
	Links               []string              `json:"Links,omitempty"`
	Aliases             []string              `json:"Aliases,omitempty"`
	NetworkID           string                `json:"NetworkID,omitempty"`
	EndpointID          string                `json:"EndpointID,omitempty"`
	Gateway             string                `json:"Gateway,omitempty"`
	IPAddress           string                `json:"IPAddress,omitempty"`
	IPPrefixLen         int                   `json:"IPPrefixLen,omitempty"`
	IPv6Gateway         string                `json:"IPv6Gateway,omitempty"`
	GlobalIPv6Address   string                `json:"GlobalIPv6Address,omitempty"`
	GlobalIPv6PrefixLen int                   `json:"GlobalIPv6PrefixLen,omitempty"`
	MacAddress          string                `json:"MacAddress,omitempty"`
	DriverOpts          map[string]string     `json:"DriverOpts,omitempty"`
}

// CreateEndpoint 返回仅包含创建时可用字段的端点配置，
// 避免把运行时字段（IP/Gateway 等）回传给创建接口导致报错。
func (e *EndpointSettings) CreateEndpoint(isDefaultNetwork bool) *EndpointSettings {
	out := &EndpointSettings{
		Links:      e.Links,
		DriverOpts: e.DriverOpts,
	}
	// 默认 bridge 网络不接受别名与静态 IP 配置。
	if !isDefaultNetwork {
		out.Aliases = e.Aliases
		if e.IPAMConfig != nil &&
			(e.IPAMConfig.IPv4Address != "" || e.IPAMConfig.IPv6Address != "" || len(e.IPAMConfig.LinkLocalIPs) > 0) {
			out.IPAMConfig = &EndpointIPAMConfig{
				IPv4Address:  e.IPAMConfig.IPv4Address,
				IPv6Address:  e.IPAMConfig.IPv6Address,
				LinkLocalIPs: e.IPAMConfig.LinkLocalIPs,
			}
		}
	}
	return out
}

// PortBinding 端口绑定。
type PortBinding struct {
	HostIP   string `json:"HostIp,omitempty"`
	HostPort string `json:"HostPort,omitempty"`
}

// RestartPolicy 重启策略。
type RestartPolicy struct {
	Name              string `json:"Name,omitempty"`
	MaximumRetryCount int    `json:"MaximumRetryCount,omitempty"`
}

// LogConfig 日志驱动配置。
type LogConfig struct {
	Type   string            `json:"Type,omitempty"`
	Config map[string]string `json:"Config,omitempty"`
}

// DeviceMapping 设备映射。
type DeviceMapping struct {
	PathOnHost        string `json:"PathOnHost"`
	PathInContainer   string `json:"PathInContainer"`
	CgroupPermissions string `json:"CgroupPermissions"`
}

// Ulimit 资源 ulimit。
type Ulimit struct {
	Name string `json:"Name"`
	Soft int64  `json:"Soft"`
	Hard int64  `json:"Hard"`
}

// Mount 创建容器时的挂载配置。
type Mount struct {
	Type          string          `json:"Type,omitempty"`
	Source        string          `json:"Source,omitempty"`
	Target        string          `json:"Target,omitempty"`
	ReadOnly      bool            `json:"ReadOnly,omitempty"`
	Consistency   string          `json:"Consistency,omitempty"`
	BindOptions   *BindOptions    `json:"BindOptions,omitempty"`
	VolumeOptions *VolumeOptions  `json:"VolumeOptions,omitempty"`
	TmpfsOptions  *TmpfsOptions   `json:"TmpfsOptions,omitempty"`
}

// BindOptions bind 挂载选项。
type BindOptions struct {
	Propagation  string `json:"Propagation,omitempty"`
	NonRecursive bool   `json:"NonRecursive,omitempty"`
}

// VolumeOptions volume 挂载选项。
type VolumeOptions struct {
	NoCopy  bool              `json:"NoCopy,omitempty"`
	Labels  map[string]string `json:"Labels,omitempty"`
	DriverConfig *Driver      `json:"DriverConfig,omitempty"`
	Subpath string            `json:"Subpath,omitempty"`
}

// TmpfsOptions tmpfs 挂载选项。
type TmpfsOptions struct {
	SizeBytes int64 `json:"SizeBytes,omitempty"`
	Mode      int   `json:"Mode,omitempty"`
}

// Driver 存储驱动。
type Driver struct {
	Name    string            `json:"Name"`
	Options map[string]string `json:"Options,omitempty"`
}

// MountPoint 容器 inspect 返回的挂载点信息。
type MountPoint struct {
	Type        string `json:"Type,omitempty"`
	Name        string `json:"Name,omitempty"`
	Source      string `json:"Source"`
	Destination string `json:"Destination"`
	Driver      string `json:"Driver,omitempty"`
	Mode        string `json:"Mode"`
	RW          bool   `json:"RW"`
	Propagation string `json:"Propagation,omitempty"`
}

// HealthConfig 健康检查配置（时间单位为纳秒，与 API 一致）。
type HealthConfig struct {
	Test        []string `json:"Test,omitempty"`
	Interval    int64    `json:"Interval,omitempty"`
	Timeout     int64    `json:"Timeout,omitempty"`
	StartPeriod int64    `json:"StartPeriod,omitempty"`
	Retries     int      `json:"Retries,omitempty"`
}

// ContainerConfig 容器配置（创建/inspect 双向使用）。
type ContainerConfig struct {
	Hostname        string              `json:"Hostname,omitempty"`
	Domainname      string              `json:"Domainname,omitempty"`
	User            string              `json:"User,omitempty"`
	AttachStdin     bool                `json:"AttachStdin,omitempty"`
	AttachStdout    bool                `json:"AttachStdout,omitempty"`
	AttachStderr    bool                `json:"AttachStderr,omitempty"`
	ExposedPorts    map[string]struct{} `json:"ExposedPorts,omitempty"`
	Tty             bool                `json:"Tty,omitempty"`
	OpenStdin       bool                `json:"OpenStdin,omitempty"`
	StdinOnce       bool                `json:"StdinOnce,omitempty"`
	Env             []string            `json:"Env,omitempty"`
	Cmd             []string            `json:"Cmd,omitempty"`
	Healthcheck     *HealthConfig       `json:"Healthcheck,omitempty"`
	ArgsEscaped     bool                `json:"ArgsEscaped,omitempty"`
	Image           string              `json:"Image"`
	Volumes         map[string]struct{} `json:"Volumes,omitempty"`
	WorkingDir      string              `json:"WorkingDir,omitempty"`
	Entrypoint      []string            `json:"Entrypoint,omitempty"`
	NetworkDisabled bool                `json:"NetworkDisabled,omitempty"`
	MacAddress      string              `json:"MacAddress,omitempty"`
	OnBuild         []string            `json:"OnBuild,omitempty"`
	Labels          map[string]string   `json:"Labels,omitempty"`
	StopSignal      string              `json:"StopSignal,omitempty"`
	StopTimeout     *int                `json:"StopTimeout,omitempty"`
	Shell           []string            `json:"Shell,omitempty"`
}

// HostConfig 容器主机配置（创建/inspect 双向使用）。
type HostConfig struct {
	Binds           []string              `json:"Binds,omitempty"`
	ContainerIDFile string                `json:"ContainerIDFile,omitempty"`
	LogConfig       LogConfig             `json:"LogConfig,omitempty"`
	NetworkMode     string                `json:"NetworkMode,omitempty"`
	PortBindings    map[string][]PortBinding `json:"PortBindings,omitempty"`
	RestartPolicy   RestartPolicy         `json:"RestartPolicy,omitempty"`
	AutoRemove      bool                  `json:"AutoRemove,omitempty"`
	VolumeDriver    string                `json:"VolumeDriver,omitempty"`
	VolumesFrom     []string              `json:"VolumesFrom,omitempty"`
	ConsoleSize     [2]uint               `json:"ConsoleSize,omitempty"`

	CapAdd         []string `json:"CapAdd,omitempty"`
	CapDrop        []string `json:"CapDrop,omitempty"`
	CgroupnsMode   string   `json:"CgroupnsMode,omitempty"`
	DNS            []string `json:"Dns,omitempty"`
	DNSOptions     []string `json:"DnsOptions,omitempty"`
	DNSSearch      []string `json:"DnsSearch,omitempty"`
	ExtraHosts     []string `json:"ExtraHosts,omitempty"`
	GroupAdd       []string `json:"GroupAdd,omitempty"`
	IpcMode        string   `json:"IpcMode,omitempty"`
	Cgroup         string   `json:"Cgroup,omitempty"`
	Links          []string `json:"Links,omitempty"`
	OomScoreAdj    int      `json:"OomScoreAdj,omitempty"`
	PidMode        string   `json:"PidMode,omitempty"`
	Privileged     bool     `json:"Privileged,omitempty"`
	PublishAllPorts bool    `json:"PublishAllPorts,omitempty"`
	ReadonlyRootfs bool     `json:"ReadonlyRootfs,omitempty"`
	SecurityOpt    []string `json:"SecurityOpt,omitempty"`
	StorageOpt     map[string]string `json:"StorageOpt,omitempty"`
	Tmpfs          map[string]string `json:"Tmpfs,omitempty"`
	UTSMode        string   `json:"UTSMode,omitempty"`
	UsernsMode     string   `json:"UsernsMode,omitempty"`
	ShmSize        int64    `json:"ShmSize,omitempty"`
	Sysctls        map[string]string `json:"Sysctls,omitempty"`
	Runtime        string   `json:"Runtime,omitempty"`
	Isolation      string   `json:"Isolation,omitempty"`
	MaskedPaths    []string `json:"MaskedPaths,omitempty"`
	ReadonlyPaths  []string `json:"ReadonlyPaths,omitempty"`

	// 资源限制。
	Memory            int64 `json:"Memory,omitempty"`
	MemorySwap        int64 `json:"MemorySwap,omitempty"`
	MemoryReservation int64 `json:"MemoryReservation,omitempty"`
	NanoCPUs          int64 `json:"NanoCpus,omitempty"`
	CPUShares         int64 `json:"CpuShares,omitempty"`
	CPUSetCPUs        string `json:"CpusetCpus,omitempty"`
	CPUSetMems        string `json:"CpusetMems,omitempty"`
	CPUQuota          int64 `json:"CpuQuota,omitempty"`
	CPUPeriod         int64 `json:"CpuPeriod,omitempty"`
	CPURealtimePeriod int64 `json:"CpuRealtimePeriod,omitempty"`
	CPURealtimeRuntime int64 `json:"CpuRealtimeRuntime,omitempty"`

	Devices []DeviceMapping `json:"Devices,omitempty"`
	Ulimits []*Ulimit       `json:"Ulimits,omitempty"`
	Mounts  []Mount         `json:"Mounts,omitempty"`
}

// ContainerState 容器运行状态。
type ContainerState struct {
	Status     string `json:"Status"`
	Running    bool   `json:"Running"`
	Paused     bool   `json:"Paused"`
	Restarting bool   `json:"Restarting"`
	OOMKilled  bool   `json:"OOMKilled"`
	Dead       bool   `json:"Dead"`
	Pid        int    `json:"Pid"`
	ExitCode   int    `json:"ExitCode"`
	Error      string `json:"Error"`
	StartedAt  string `json:"StartedAt"`
	FinishedAt string `json:"FinishedAt"`
}

// NetworkSettings 网络设置。
type NetworkSettings struct {
	Networks map[string]*EndpointSettings `json:"Networks"`
}

// ContainerSummary 容器列表条目。
type ContainerSummary struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	ImageID string            `json:"ImageID"`
	Command string            `json:"Command"`
	Created int64             `json:"Created"`
	Ports   []Port            `json:"Ports"`
	Labels  map[string]string `json:"Labels"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`

	HostConfig struct {
		NetworkMode string `json:"NetworkMode"`
	} `json:"HostConfig"`
	NetworkSettings struct {
		Networks map[string]*EndpointSettings `json:"Networks"`
	} `json:"NetworkSettings"`
}

// ContainerInspect 容器详细信息。
type ContainerInspect struct {
	ID              string            `json:"Id"`
	Name            string            `json:"Name"`
	Created         string            `json:"Created"`
	Path            string            `json:"Path"`
	Args            []string          `json:"Args"`
	State           ContainerState    `json:"State"`
	Image           string            `json:"Image"`
	ResolvConfPath  string            `json:"ResolvConfPath"`
	HostnamePath    string            `json:"HostnamePath"`
	HostsPath       string            `json:"HostsPath"`
	LogPath         string            `json:"LogPath"`
	RestartCount    int               `json:"RestartCount"`
	Driver          string            `json:"Driver"`
	Platform        string            `json:"Platform"`
	Config          ContainerConfig   `json:"Config"`
	HostConfig      HostConfig        `json:"HostConfig"`
	NetworkSettings NetworkSettings   `json:"NetworkSettings"`
	Mounts          []MountPoint      `json:"Mounts"`
}

// NetworkingConfig 创建容器时的网络配置。
type NetworkingConfig struct {
	EndpointsConfig map[string]*EndpointSettings `json:"EndpointsConfig,omitempty"`
}

// ContainerCreateRequest 创建容器请求体。
type ContainerCreateRequest struct {
	Config           ContainerConfig   `json:"Config"`
	HostConfig       HostConfig        `json:"HostConfig"`
	NetworkingConfig *NetworkingConfig `json:"NetworkingConfig,omitempty"`
}

// ContainerCreateResponse 创建容器响应。
type ContainerCreateResponse struct {
	ID       string   `json:"Id"`
	Warnings []string `json:"Warnings"`
}

// ImageSummary 镜像列表条目。
type ImageSummary struct {
	ID          string            `json:"Id"`
	ParentID    string            `json:"ParentId"`
	RepoTags    []string          `json:"RepoTags"`
	RepoDigests []string          `json:"RepoDigests"`
	Created     int64             `json:"Created"`
	Size        int64             `json:"Size"`
	SharedSize  int64             `json:"SharedSize"`
	Labels      map[string]string `json:"Labels"`
	Containers  int64             `json:"Containers"`
}

// ImageInspect 镜像详细信息。
type ImageInspect struct {
	ID              string            `json:"Id"`
	RepoTags        []string          `json:"RepoTags"`
	RepoDigests     []string          `json:"RepoDigests"`
	Parent          string            `json:"Parent"`
	Comment         string            `json:"Comment"`
	Created         string            `json:"Created"`
	Container       string            `json:"Container"`
	Author          string            `json:"Author"`
	Config          *ContainerConfig  `json:"Config"`
	Architecture    string            `json:"Architecture"`
	Variant         string            `json:"Variant,omitempty"`
	Os              string            `json:"Os"`
	OsVersion       string            `json:"OsVersion,omitempty"`
	Size            int64             `json:"Size"`
	VirtualSize     int64             `json:"VirtualSize"`
	GraphDriver     struct {
		Name string            `json:"Name"`
		Data map[string]string `json:"Data"`
	} `json:"GraphDriver"`
	RootFS struct {
		Type   string   `json:"Type"`
		Layers []string `json:"Layers"`
	} `json:"RootFS"`
}

// ImageDeleteResponseItem 删除镜像结果条目。
type ImageDeleteResponseItem struct {
	Untagged string `json:"Untagged"`
	Deleted  string `json:"Deleted"`
}

// Volume 存储卷信息。
type Volume struct {
	Name       string            `json:"Name"`
	Driver     string            `json:"Driver"`
	Mountpoint string            `json:"Mountpoint"`
	CreatedAt  string            `json:"CreatedAt"`
	Status     map[string]any    `json:"Status,omitempty"`
	Labels     map[string]string `json:"Labels"`
	Scope      string            `json:"Scope"`
	Options    map[string]string `json:"Options,omitempty"`
	UsageData  *VolumeUsageData  `json:"UsageData,omitempty"`
}

// VolumeUsageData 存储卷用量信息。
type VolumeUsageData struct {
	Size     int64 `json:"Size"`
	RefCount int64 `json:"RefCount"`
}

// VolumeListResponse 存储卷列表响应。
type VolumeListResponse struct {
	Volumes  []*Volume `json:"Volumes"`
	Warnings []string  `json:"Warnings"`
}

// VersionResponse `GET /version` 响应。
type VersionResponse struct {
	Platform struct {
		Name string `json:"Name"`
	} `json:"Platform"`
	Version       string `json:"Version"`
	APIVersion    string `json:"ApiVersion"`
	MinAPIVersion string `json:"MinAPIVersion"`
	GitCommit     string `json:"GitCommit"`
	GoVersion     string `json:"GoVersion"`
	Os            string `json:"Os"`
	Arch          string `json:"Arch"`
	KernelVersion string `json:"KernelVersion"`
	Experimental  bool   `json:"Experimental"`
	BuildTime     string `json:"BuildTime"`
}

// PullProgress 拉取镜像流式响应条目。
type PullProgress struct {
	Status         string `json:"status"`
	Progress       string `json:"progress,omitempty"`
	ProgressDetail struct {
		Current int64 `json:"current"`
		Total   int64 `json:"total"`
	} `json:"progressDetail,omitempty"`
	ID    string `json:"id,omitempty"`
	Error string `json:"error,omitempty"`
	ErrorDetail struct {
		Code    int    `json:"code,omitempty"`
		Message string `json:"message,omitempty"`
	} `json:"errorDetail,omitempty"`
}
