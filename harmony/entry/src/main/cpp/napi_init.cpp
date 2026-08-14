#include "napi/native_api.h"
#include "libnebula.h"

#include <cstdlib>

// 对应 Go 侧 NebulaRun
static napi_value NapiRun(napi_env env, napi_callback_info info) {
    NebulaRun();
    napi_value result = nullptr;
    napi_get_undefined(env, &result);
    return result;
}

// 对应 Go 侧 NebulaSetDataDir
static napi_value NapiSetDataDir(napi_env env, napi_callback_info info) {
    size_t argc = 1;
    napi_value args[1] = {nullptr};
    napi_get_cb_info(env, info, &argc, args, nullptr, nullptr);

    napi_value result = nullptr;
    napi_get_undefined(env, &result);
    if (argc < 1) {
        return result;
    }

    size_t len = 0;
    napi_get_value_string_utf8(env, args[0], nullptr, 0, &len);
    char* buf = new char[len + 1];
    napi_get_value_string_utf8(env, args[0], buf, len + 1, &len);
    NebulaSetDataDir(buf);
    delete[] buf;
    return result;
}

// 对应 Go 侧 NebulaGetOpuiUrl
static napi_value NapiGetOpuiUrl(napi_env env, napi_callback_info info) {
    char* url = NebulaGetOpuiUrl();
    napi_value result = nullptr;
    napi_create_string_utf8(env, url != nullptr ? url : "", NAPI_AUTO_LENGTH, &result);
    if (url != nullptr) {
        std::free(url);
    }
    return result;
}

// 对应 Go 侧 NebulaSetDeviceInfo
static napi_value NapiSetDeviceInfo(napi_env env, napi_callback_info info) {
    size_t argc = 1;
    napi_value args[1] = {nullptr};
    napi_get_cb_info(env, info, &argc, args, nullptr, nullptr);

    napi_value result = nullptr;
    napi_get_undefined(env, &result);
    if (argc < 1) {
        return result;
    }

    size_t len = 0;
    napi_get_value_string_utf8(env, args[0], nullptr, 0, &len);
    char* buf = new char[len + 1];
    napi_get_value_string_utf8(env, args[0], buf, len + 1, &len);
    NebulaSetDeviceInfo(buf);
    delete[] buf;
    return result;
}

// 对应 Go 侧 NebulaPollNotification
static napi_value NapiPollNotification(napi_env env, napi_callback_info info) {
    char* json = NebulaPollNotification();
    napi_value result = nullptr;
    napi_create_string_utf8(env, json != nullptr ? json : "", NAPI_AUTO_LENGTH, &result);
    if (json != nullptr) {
        std::free(json);
    }
    return result;
}

// 对应 Go 侧 NebulaUpdateBattery
static napi_value NapiUpdateBattery(napi_env env, napi_callback_info info) {
    size_t argc = 2;
    napi_value args[2] = {nullptr};
    napi_get_cb_info(env, info, &argc, args, nullptr, nullptr);

    napi_value result = nullptr;
    napi_get_undefined(env, &result);
    if (argc < 2) {
        return result;
    }

    int32_t level = 0;
    bool charging = false;
    napi_get_value_int32(env, args[0], &level);
    napi_get_value_bool(env, args[1], &charging);
    NebulaUpdateBattery(level, charging ? 1 : 0);
    return result;
}

EXTERN_C_START
static napi_value Init(napi_env env, napi_value exports) {
    napi_property_descriptor desc[] = {
        {"run", nullptr, NapiRun, nullptr, nullptr, nullptr, napi_default, nullptr},
        {"setDataDir", nullptr, NapiSetDataDir, nullptr, nullptr, nullptr, napi_default, nullptr},
        {"getOpuiUrl", nullptr, NapiGetOpuiUrl, nullptr, nullptr, nullptr, napi_default, nullptr},
        {"setDeviceInfo", nullptr, NapiSetDeviceInfo, nullptr, nullptr, nullptr, napi_default, nullptr},
        {"updateBattery", nullptr, NapiUpdateBattery, nullptr, nullptr, nullptr, napi_default, nullptr},
        {"pollNotification", nullptr, NapiPollNotification, nullptr, nullptr, nullptr, napi_default, nullptr},
    };
    napi_define_properties(env, exports, sizeof(desc) / sizeof(desc[0]), desc);
    return exports;
}
EXTERN_C_END

static napi_module nebulaModule = {
    .nm_version = 1,
    .nm_flags = 0,
    .nm_filename = nullptr,
    .nm_register_func = Init,
    .nm_modname = "entry",
    .nm_priv = nullptr,
    .reserved = {0},
};

extern "C" __attribute__((constructor)) void RegisterNebulaModule(void) {
    napi_module_register(&nebulaModule);
}
