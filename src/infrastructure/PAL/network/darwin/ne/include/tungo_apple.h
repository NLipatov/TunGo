#ifndef TUNGO_APPLE_H
#define TUNGO_APPLE_H

#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

// Locates the UTUN control socket owned by the packet tunnel provider process.
// Returns -1 when no descriptor can be identified.
int32_t tungo_find_utun_fd(void);

// Returned error strings and network settings are allocated by the Go backend
// and must be released with tungo_free.
char *tungo_network_settings(char **output);
char *tungo_start(int32_t tunnel_file_descriptor);
char *tungo_wait_ready(int64_t timeout_milliseconds);
char *tungo_stop(void);
void tungo_free(void *pointer);

#ifdef __cplusplus
}
#endif

#endif
