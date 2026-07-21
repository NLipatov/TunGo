#ifndef TUNGO_APPLE_H
#define TUNGO_APPLE_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

enum tungo_state {
    TUNGO_STATE_STARTING = 1,
    TUNGO_STATE_RUNNING = 2,
    TUNGO_STATE_STOPPED = 3,
    TUNGO_STATE_FAILED = 4,
};

// Locates the UTUN control socket owned by the packet tunnel provider process.
// Returns -1 when no descriptor can be identified.
int32_t tungo_find_utun_fd(void);

// Returned error strings, plans, and status details are allocated by the Go
// backend and must be released with tungo_free.
char *tungo_plan(char **output);
char *tungo_start(
    int32_t tunnel_file_descriptor,
    uint64_t *output_handle
);
char *tungo_wait_ready(uint64_t handle, int64_t timeout_milliseconds);
char *tungo_stop(uint64_t handle);
char *tungo_pause(uint64_t handle);
char *tungo_restart(uint64_t handle);
char *tungo_status(uint64_t handle, int32_t *output_state, char **output_detail);
void tungo_free(void *pointer);

#ifdef __cplusplus
}
#endif

#endif
