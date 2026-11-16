package service

import (
	"context"
	"errors"

	"wakepilot/internal/device"
	"wakepilot/internal/network"
)

// NetworkResolver is the part of network.Service the services need.
type NetworkResolver interface {
	Resolve(network.ResolveRequest) (network.Resolved, error)
}

// DeviceView is a device plus its latest availability.
type DeviceView struct {
	device.Device
	Availability Availability `json:"availability"`
}

type DeviceService struct {
	repo        device.Repository
	net         NetworkResolver
	monitor     *Monitor
	defaultPort int
}

func NewDeviceService(repo device.Repository, n NetworkResolver, m *Monitor, defaultPort int) *DeviceService {
	return &DeviceService{repo: repo, net: n, monitor: m, defaultPort: defaultPort}
}

func (s *DeviceService) view(d device.Device) DeviceView {
	return DeviceView{Device: d, Availability: s.monitor.Get(d)}
}

// validate normalises in and verifies the network settings can be resolved.
func (s *DeviceService) validate(in *device.Input) error {
	if err := in.Normalize(s.defaultPort); err != nil {
		return err
	}
	if _, err := s.net.Resolve(network.ResolveRequest{Interface: in.Interface, Broadcast: in.Broadcast, DeviceIP: in.IP}); err != nil {
		if !network.IsConfigError(err) {
			return err
		}
		field := "broadcast"
		if errors.Is(err, network.ErrInterfaceNotFound) {
			field = "interface"
		} else if errors.Is(err, network.ErrInvalidAddress) && in.IP != "" && !errors.Is(err, network.ErrBroadcastNotAllowed) {
			field = "ip"
		}
		return device.ValidationError{field: err.Error()}
	}
	return nil
}

func (s *DeviceService) Create(ctx context.Context, in device.Input) (DeviceView, error) {
	if err := s.validate(&in); err != nil {
		return DeviceView{}, err
	}
	d, err := s.repo.Create(ctx, toDevice(0, in))
	if err != nil {
		return DeviceView{}, mapDuplicate(err)
	}
	s.monitor.Trigger()
	return s.view(d), nil
}

func (s *DeviceService) Get(ctx context.Context, id int64) (DeviceView, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		return DeviceView{}, err
	}
	return s.view(d), nil
}

func (s *DeviceService) List(ctx context.Context) ([]DeviceView, error) {
	ds, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]DeviceView, len(ds))
	for i, d := range ds {
		out[i] = s.view(d)
	}
	return out, nil
}

func (s *DeviceService) Update(ctx context.Context, id int64, in device.Input) (DeviceView, error) {
	if _, err := s.repo.Get(ctx, id); err != nil {
		return DeviceView{}, err
	}
	if err := s.validate(&in); err != nil {
		return DeviceView{}, err
	}
	d, err := s.repo.Update(ctx, toDevice(id, in))
	if err != nil {
		return DeviceView{}, mapDuplicate(err)
	}
	s.monitor.Forget(id)
	s.monitor.Trigger()
	return s.view(d), nil
}

func (s *DeviceService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.monitor.Forget(id)
	return nil
}

// Check probes the device now (on demand, regardless of the periodic monitor).
func (s *DeviceService) Check(ctx context.Context, id int64) (DeviceView, error) {
	d, err := s.repo.Get(ctx, id)
	if err != nil {
		return DeviceView{}, err
	}
	s.monitor.Check(ctx, d)
	return s.view(d), nil
}

func toDevice(id int64, in device.Input) device.Device {
	return device.Device{ID: id, Name: in.Name, MAC: in.MAC, IP: in.IP, Broadcast: in.Broadcast,
		Port: in.Port, Interface: in.Interface, Description: in.Description}
}

func mapDuplicate(err error) error {
	if errors.Is(err, device.ErrDuplicateMAC) {
		return device.ValidationError{"mac": err.Error()}
	}
	return err
}
