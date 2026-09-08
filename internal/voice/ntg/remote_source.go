package ntg

type RemoteSource struct {
	Ssrc   uint32
	State  StreamStatus
	Device StreamDevice
}
