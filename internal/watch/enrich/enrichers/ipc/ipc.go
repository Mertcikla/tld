package ipc

import (
	"github.com/mertcikla/tld/internal/watch/enrich"
	"github.com/mertcikla/tld/internal/watch/enrich/enrichers/pattern"
)

func All() []enrich.Enricher { return pattern.FromSpecs(Specs()) }

func Specs() []pattern.Spec {
	return []pattern.Spec{
		spec("go.unix_socket", "Go Unix Domain Sockets", "go", "net", "AF_UNIX", "ipc.socket_path", "connects_to_socket"),
		spec("python.unix_socket", "Python Unix Domain Sockets", "python", "socket", "AF_UNIX", "ipc.socket_path", "connects_to_socket"),
		spec("rust.unix_socket", "Rust Unix Domain Sockets", "rust", "tokio", "UnixStream", "ipc.socket_path", "connects_to_socket"),
		spec("cpp.unix_socket", "C++ Unix Domain Sockets", "cpp", "sys/socket.h", "AF_UNIX", "ipc.socket_path", "connects_to_socket"),
		spec("go.dbus", "Go D-Bus", "go", "github.com/godbus/dbus", "org.freedesktop", "ipc.dbus_interface", "exposes_dbus_service"),
		spec("python.dbus", "Python D-Bus", "python", "dbus-python", "org.freedesktop", "ipc.dbus_interface", "exposes_dbus_service"),
		spec("rust.dbus", "Rust D-Bus", "rust", "zbus", "org.freedesktop", "ipc.dbus_interface", "exposes_dbus_service"),
		spec("cpp.dbus", "C++ D-Bus", "cpp", "sdbus-c++", "org.freedesktop", "ipc.dbus_interface", "exposes_dbus_service"),
		spec("ts.named_pipes", "TypeScript Named Pipes", "typescript", "net", "\\\\.\\pipe\\", "ipc.socket_path", "connects_to_socket"),
		spec("go.named_pipes", "Go Named Pipes", "go", "winio", "\\\\.\\pipe\\", "ipc.socket_path", "connects_to_socket"),
		spec("python.named_pipes", "Python Named Pipes", "python", "pywin32", "\\\\.\\pipe\\", "ipc.socket_path", "connects_to_socket"),
		spec("cpp.named_pipes", "C++ Named Pipes", "cpp", "windows.h", "CreateNamedPipe", "ipc.socket_path", "connects_to_socket"),
		spec("go.grpc_uds", "Go gRPC over UDS", "go", "google.golang.org/grpc", "grpc.WithContextDialer", "ipc.socket_path", "connects_to_socket"),
		spec("python.grpc_uds", "Python gRPC over UDS", "python", "grpcio", "unix:", "ipc.socket_path", "connects_to_socket"),
		spec("ts.grpc_uds", "TypeScript gRPC over UDS", "typescript", "@grpc/grpc-js", "unix:", "ipc.socket_path", "connects_to_socket"),
		spec("cpp.grpc_uds", "C++ gRPC over UDS", "cpp", "grpc", "unix:", "ipc.socket_path", "connects_to_socket"),
		spec("go.dev_node", "Go /dev device nodes", "go", "os", "/dev/", "kernel.device_node", "reads_device"),
		spec("python.dev_node", "Python /dev device nodes", "python", "os", "/dev/", "kernel.device_node", "reads_device"),
		spec("rust.dev_node", "Rust /dev device nodes", "rust", "std", "/dev/", "kernel.device_node", "reads_device"),
		spec("cpp.dev_node", "C++ /dev device nodes", "cpp", "fcntl.h", "/dev/", "kernel.device_node", "reads_device"),
		spec("go.sysfs_procfs", "Go sysfs / procfs", "go", "os", "/proc/", "kernel.device_node", "reads_device"),
		spec("python.sysfs_procfs", "Python sysfs / procfs", "python", "os", "/sys/", "kernel.device_node", "reads_device"),
		spec("rust.sysfs_procfs", "Rust sysfs / procfs", "rust", "std", "/proc/", "kernel.device_node", "reads_device"),
		spec("cpp.sysfs_procfs", "C++ sysfs / procfs", "cpp", "fstream", "/sys/", "kernel.device_node", "reads_device"),
		spec("go.ebpf", "Go eBPF", "go", "github.com/cilium/ebpf", "kprobe", "kernel.device_node", "reads_device"),
		spec("python.ebpf", "Python eBPF", "python", "bcc", "BPF(", "kernel.device_node", "reads_device"),
		spec("rust.ebpf", "Rust eBPF", "rust", "aya", "tracepoint", "kernel.device_node", "reads_device"),
		spec("cpp.ebpf", "C++ eBPF", "cpp", "libbpf", "uprobe", "kernel.device_node", "reads_device"),
	}
}

func spec(id, name, language, dependency, token, factType, relationship string) pattern.Spec {
	return pattern.Spec{
		ID:           id,
		Name:         name,
		Category:     "ipc",
		Languages:    []string{language},
		Mode:         enrich.ActivationImportOrDependency,
		Triggers:     []enrich.ActivationSignal{{Kind: enrich.SignalDependency, Value: dependency}, {Kind: enrich.SignalImport, Value: dependency}},
		FactType:     factType,
		Relationship: relationship,
		SourceTokens: []string{token},
		Tags:         []string{"ipc:" + id},
		Attributes:   map[string]string{"dependency": dependency, "language": language},
	}
}
