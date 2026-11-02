//go:build android

package vpn

/*
#include <jni.h>
*/
import "C"

import (
	"unsafe"

	"github.com/mehran9675/giode/android"
)

// The JNI glue: calls into com.middlelan.app.VpnBridge and its native
// methods. All logic lives in control_android.go.

const bridgeClass = "com/middlelan/app/VpnBridge"

type jniBridge struct{}

func init() { bridge = jniBridge{} }

func (jniBridge) prepare() (bool, error) {
	return android.CallBool(bridgeClass, "prepare", "(Landroid/content/Context;)Z", android.Context())
}

func (jniBridge) isPrepared() (bool, error) {
	return android.CallBool(bridgeClass, "isPrepared", "(Landroid/content/Context;)Z", android.Context())
}

func (jniBridge) start() (bool, error) {
	return android.CallBool(bridgeClass, "start", "(Landroid/content/Context;)Z", android.Context())
}

func (jniBridge) stop() error {
	return android.CallVoid(bridgeClass, "stop", "(Landroid/content/Context;)V", android.Context())
}

func (jniBridge) ownerUID(proto int, srcIP string, srcPort int, dstIP string, dstPort int) (int, error) {
	return android.CallInt(bridgeClass, "ownerUid", "(ILjava/lang/String;ILjava/lang/String;I)I",
		int32(proto), srcIP, int32(srcPort), dstIP, int32(dstPort))
}

func (jniBridge) appLabel(uid int) (string, error) {
	return android.CallString(bridgeClass, "appLabel", "(Landroid/content/Context;I)Ljava/lang/String;",
		android.Context(), int32(uid))
}

//export Java_com_middlelan_app_VpnBridge_nativeStart
func Java_com_middlelan_app_VpnBridge_nativeStart(env *C.JNIEnv, cls C.jclass, fd C.jint, dns C.jstring) C.jboolean {
	if nativeStart(int(fd), android.GoString(unsafe.Pointer(env), uintptr(dns))) != nil {
		return 0
	}
	return 1
}

//export Java_com_middlelan_app_VpnBridge_nativeStop
func Java_com_middlelan_app_VpnBridge_nativeStop(env *C.JNIEnv, cls C.jclass) {
	nativeStop()
}

//export Java_com_middlelan_app_VpnBridge_nativeSetDns
func Java_com_middlelan_app_VpnBridge_nativeSetDns(env *C.JNIEnv, cls C.jclass, dns C.jstring) {
	nativeSetDNS(android.GoString(unsafe.Pointer(env), uintptr(dns)))
}
