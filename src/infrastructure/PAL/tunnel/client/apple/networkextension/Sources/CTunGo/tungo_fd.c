#include "tungo_apple.h"

#include <string.h>
#include <sys/ioctl.h>
#include <sys/kern_control.h>
#include <sys/socket.h>

int32_t tungo_find_utun_fd(void) {
    struct ctl_info control_info = {0};
    strlcpy(control_info.ctl_name, "com.apple.net.utun_control", sizeof(control_info.ctl_name));

    for (int32_t fd = 0; fd <= 1024; ++fd) {
        struct sockaddr_ctl address = {0};
        socklen_t address_length = sizeof(address);
        if (getpeername(fd, (struct sockaddr *)&address, &address_length) != 0) {
            continue;
        }
        if (address.sc_family != AF_SYSTEM) {
            continue;
        }
        if (control_info.ctl_id == 0 && ioctl(fd, CTLIOCGINFO, &control_info) != 0) {
            continue;
        }
        if (address.sc_id == control_info.ctl_id) {
            return fd;
        }
    }
    return -1;
}
