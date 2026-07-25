#ifndef TUNGO_APPLE_H
#define TUNGO_APPLE_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// Locates the UTUN control socket owned by the packet tunnel provider process.
// Returns -1 when no descriptor can be identified.
int32_t tungo_find_utun_fd(void);

// An opaque reference to a Go controller. Zero is never a valid handle.
typedef uintptr_t tungo_controller_handle_t;

// Creates a controller owned by the caller.
tungo_controller_handle_t tungo_controller_create(void);

// Stops the controller if necessary and releases its handle.
// Invalid and previously released handles are ignored.
void tungo_controller_destroy(tungo_controller_handle_t controller);

// Returned error strings and network settings are allocated by the Go backend
// and must be released with tungo_free.
char *tungo_network_settings(char **output);
char *tungo_start(
    tungo_controller_handle_t controller,
    int32_t tunnel_file_descriptor
);
char *tungo_wait_ready(
    tungo_controller_handle_t controller,
    int64_t timeout_milliseconds
);
char *tungo_stop(tungo_controller_handle_t controller);

// Releases memory returned by the TunGo C ABI.
// Passing any other pointer results in undefined behavior.
void tungo_free(void *pointer);

#ifdef __cplusplus
}
#endif

#endif
