package service

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

// SystemdManager is a client for the systemd Manager D-Bus interface.
//
// It binds to the well-known bus name "org.freedesktop.systemd1" at the
// object path "/org/freedesktop/systemd1", which exports the
// org.freedesktop.systemd1.Manager interface.
//
// This interface provides methods for controlling and introspecting systemd units.
type SystemdManager struct {
	obj dbus.BusObject // Proxy object bound to /org/freedesktop/systemd1.
}

// NewSystemdManager returns a SystemdManager bound to the systemd Manager
// interface at the object path "/org/freedesktop/systemd1".
//
// Parameters:
//   - conn — active D-Bus connection (typically to the system bus).
//
// Returns:
//   - *SystemdManager — a manager instance for interacting with systemd units.
func NewSystemdManager(conn *dbus.Conn) *SystemdManager {
	return &SystemdManager{
		obj: conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1"),
	}
}

// prop represents a single systemd unit property in the D-Bus signature (s, v),
// i.e. a (string, variant) tuple used in Manager method calls like StartTransientUnit.
type prop struct {
	Name  string
	Value dbus.Variant
}

// StartTransientUnit creates and starts a transient `.service` unit.
//
// It invokes the org.freedesktop.systemd1.Manager.StartTransientUnit method:
//
//	StartTransientUnit(
//	    in  s name,          // unit name (must include ".service" suffix)
//	    in  s mode,          // job mode (e.g. "replace", "fail")
//	    in  a(sv) properties // array of (string, variant) property assignments
//	    in  a(sa(sv)) aux,   // auxiliary units (usually empty)
//	    out o job            // object path for the queued systemd job
//	)
//
// The transient unit is defined entirely in-memory and does not persist to disk.
//
// Properties set here include:
//   - ExecStart (a(sasb)) → command to execute
//   - Restart → restart policy (e.g. "always")
//   - RestartUSec → restart delay in microseconds
//
// See: https://www.freedesktop.org/wiki/Software/systemd/dbus/#starttransientunit
//
// Note: This D-Bus method is **asynchronous by design** —
//   - it does not wait for the unit to start or complete.
//   - It only registers the job and returns immediately with a job object path.
//   - To track job progress or outcome, you must subscribe to systemd signals
//     (e.g., JobRemoved) or poll unit status separately.
func (m *SystemdManager) StartTransientUnit(unit string, path string, argv []string, restart string, usec uint64) (dbus.ObjectPath, error) {
	// Build property list (a[s,v]).
	// ExecStart expects an array of structs with fields:
	//   s (path), as (argv[]), b (ignore-failure).
	props := []prop{
		{
			Name: "ExecStart",
			Value: dbus.MakeVariant([]struct {
				Path          string
				Argv          []string
				IgnoreFailure bool
			}{
				{path, argv, true},
			}),
		},
		{"Restart", dbus.MakeVariant(restart)},
		{"RestartUSec", dbus.MakeVariant(usec)},
	}

	// Auxiliary units array: a(sa(sv))
	// Typically unused, so an empty slice is passed.
	aux := []struct {
		Name  string
		Props []prop
	}{}

	// Call StartTransientUnit on the Manager interface.
	var jobPath dbus.ObjectPath
	call := m.obj.Call("org.freedesktop.systemd1.Manager.StartTransientUnit", 0, unit, "replace", props, aux)
	if call.Err != nil {
		return jobPath, fmt.Errorf("StartTransientUnit %q call: %w", unit, call.Err)
	}

	// Parse returned object path for the queued job (e.g. /org/freedesktop/systemd1/job/123).
	if err := call.Store(&jobPath); err != nil {
		return jobPath, fmt.Errorf("StartTransientUnit %q store: %w", unit, err)
	}

	return jobPath, nil
}

// StopUnit requests systemd to stop an active unit.
//
// It invokes the org.freedesktop.systemd1.Manager.StopUnit method:
//
//	StopUnit(
//	    in  s name, // unit name (e.g. "foo.service")
//	    in  s mode, // job mode ("replace", "fail", "ignore-dependencies")
//	    out o job   // object path for the queued systemd job
//	)
//
// See: https://www.freedesktop.org/wiki/Software/systemd/dbus/#stopunit
//
// Note: This D-Bus method is **asynchronous by design** —
//   - it does not wait for the unit to stop or for the job to finish.
//   - It only queues the stop request and returns a job object path.
//   - To monitor the outcome, listen for the JobRemoved signal or poll unit state.
func (m *SystemdManager) StopUnit(unit string) (dbus.ObjectPath, error) {
	var jobPath dbus.ObjectPath
	call := m.obj.Call("org.freedesktop.systemd1.Manager.StopUnit", 0, unit, "replace")
	if call.Err != nil {
		return jobPath, fmt.Errorf("StopUnit %q call: %w", unit, call.Err)
	}
	if err := call.Store(&jobPath); err != nil {
		return jobPath, fmt.Errorf("StopUnit %q store: %w", unit, err)
	}
	return jobPath, nil
}

// UnitStatus mirrors the tuple returned by Manager.ListUnits:
// (s name, s desc, s load, s active, s sub, s followed, o path, u jobId, s jobType, o jobPath)
type UnitStatus struct {
	Name        string
	Description string
	LoadState   string
	ActiveState string
	SubState    string
	Followed    string
	Path        dbus.ObjectPath
	JobId       uint32
	JobType     string
	JobPath     dbus.ObjectPath
}

// ListUnits fetches all known units from systemd.
func (m *SystemdManager) ListUnits() ([]UnitStatus, error) {
	var units []UnitStatus
	call := m.obj.Call("org.freedesktop.systemd1.Manager.ListUnits", 0)
	if call.Err != nil {
		return nil, fmt.Errorf("ListUnits call: %w", call.Err)
	}
	if err := call.Store(&units); err != nil {
		return nil, fmt.Errorf("ListUnits store: %w", err)
	}
	return units, nil
}
