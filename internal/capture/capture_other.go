//go:build !windows && !darwin && !linux

package capture

func platformBackend() Backend { return unsupportedBackend{} }
