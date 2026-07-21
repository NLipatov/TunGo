#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPOSITORY_ROOT="$(cd "${SCRIPT_DIR}/../../../../../.." && pwd)"
OUTPUT_ROOT="${TUNGO_APPLE_BUILD_DIR:-${SCRIPT_DIR}/build}"
DEPLOYMENT_TARGET="${TUNGO_APPLE_DEPLOYMENT_TARGET:-15.0}"
BRIDGE_PACKAGE="./infrastructure/PAL/tunnel/client/apple/carchive"

build_archive() {
    local goos="$1"
    local goarch="$2"
    local sdk="$3"
    local clang_target="$4"
    local output="$5"
    local sdk_path
    local compiler

    sdk_path="$(xcrun --sdk "${sdk}" --show-sdk-path)"
    compiler="$(xcrun --sdk "${sdk}" --find clang)"
    mkdir -p "$(dirname "${output}")"

    (
        cd "${REPOSITORY_ROOT}"
        CGO_ENABLED=1 \
        GOCACHE="${OUTPUT_ROOT}/go-cache" \
        GOOS="${goos}" \
        GOARCH="${goarch}" \
        CC="${compiler}" \
        CGO_CFLAGS="-isysroot ${sdk_path} -target ${clang_target}" \
        CGO_LDFLAGS="-isysroot ${sdk_path} -target ${clang_target}" \
        go build -trimpath -buildmode=c-archive -o "${output}" "${BRIDGE_PACKAGE}"
    )
}

build_macos() {
    local arm64_archive="${OUTPUT_ROOT}/macos/arm64/libtungo_apple.a"
    local amd64_archive="${OUTPUT_ROOT}/macos/x86_64/libtungo_apple.a"
    local universal_archive="${OUTPUT_ROOT}/macos/universal/libtungo_apple.a"

    build_archive darwin arm64 macosx "arm64-apple-macos${DEPLOYMENT_TARGET}" "${arm64_archive}"
    build_archive darwin amd64 macosx "x86_64-apple-macos${DEPLOYMENT_TARGET}" "${amd64_archive}"
    mkdir -p "$(dirname "${universal_archive}")"
    lipo -create "${arm64_archive}" "${amd64_archive}" -output "${universal_archive}"
}

build_ios() {
    build_archive ios arm64 iphoneos \
        "arm64-apple-ios${DEPLOYMENT_TARGET}" \
        "${OUTPUT_ROOT}/ios/device/libtungo_apple.a"

    build_archive ios arm64 iphonesimulator \
        "arm64-apple-ios${DEPLOYMENT_TARGET}-simulator" \
        "${OUTPUT_ROOT}/ios/simulator-arm64/libtungo_apple.a"
    build_archive ios amd64 iphonesimulator \
        "x86_64-apple-ios${DEPLOYMENT_TARGET}-simulator" \
        "${OUTPUT_ROOT}/ios/simulator-x86_64/libtungo_apple.a"
    mkdir -p "${OUTPUT_ROOT}/ios/simulator"
    lipo -create \
        "${OUTPUT_ROOT}/ios/simulator-arm64/libtungo_apple.a" \
        "${OUTPUT_ROOT}/ios/simulator-x86_64/libtungo_apple.a" \
        -output "${OUTPUT_ROOT}/ios/simulator/libtungo_apple.a"
}

build_xcframework() {
    build_macos
    build_ios
    xcodebuild -create-xcframework \
        -library "${OUTPUT_ROOT}/macos/universal/libtungo_apple.a" -headers "${SCRIPT_DIR}/include" \
        -library "${OUTPUT_ROOT}/ios/device/libtungo_apple.a" -headers "${SCRIPT_DIR}/include" \
        -library "${OUTPUT_ROOT}/ios/simulator/libtungo_apple.a" -headers "${SCRIPT_DIR}/include" \
        -output "${OUTPUT_ROOT}/TunGoCore.xcframework"
}

case "${1:-macos}" in
    macos)
        build_macos
        ;;
    ios)
        build_ios
        ;;
    xcframework)
        build_xcframework
        ;;
    *)
        echo "usage: $0 [macos|ios|xcframework]" >&2
        exit 2
        ;;
esac
