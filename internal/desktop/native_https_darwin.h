//go:build darwin && !ios

#ifndef GUL_NATIVE_HTTPS_H
#define GUL_NATIVE_HTTPS_H

#include <stdbool.h>
#include <stdint.h>

bool gulInstallNativeHTTPS(void *window, const char *origin, const uint8_t *pin, const char *script,
                           int64_t validFrom, int64_t validUntil);
bool gulProbeNativeHTTPS(const char *origin, const char *candidate, const uint8_t *pin,
                         const uint8_t *der, int derLength, int64_t validFrom, int64_t validUntil);
bool gulProbeDelegateForwarding(void);
void gulStopNativeApplication(void);

#endif
