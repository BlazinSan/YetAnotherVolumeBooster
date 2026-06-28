//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

const (
	coinitApartmentThreaded = 0x2
	clsctxAll               = 0x17
	eRender                 = 0
	eConsole                = 0
	eMultimedia             = 1
	rpcEChangedMode         = 0x80010106
)

type guid struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type iMMDeviceEnumerator struct{ lpVtbl *iMMDeviceEnumeratorVtbl }
type iMMDeviceEnumeratorVtbl struct {
	QueryInterface                         uintptr
	AddRef                                 uintptr
	Release                                uintptr
	EnumAudioEndpoints                     uintptr
	GetDefaultAudioEndpoint                uintptr
	GetDevice                              uintptr
	RegisterEndpointNotificationCallback   uintptr
	UnregisterEndpointNotificationCallback uintptr
}

type iMMDevice struct{ lpVtbl *iMMDeviceVtbl }
type iMMDeviceVtbl struct {
	QueryInterface    uintptr
	AddRef            uintptr
	Release           uintptr
	Activate          uintptr
	OpenPropertyStore uintptr
	GetID             uintptr
	GetState          uintptr
}

type iAudioEndpointVolume struct{ lpVtbl *iAudioEndpointVolumeVtbl }
type iAudioEndpointVolumeVtbl struct {
	QueryInterface                uintptr
	AddRef                        uintptr
	Release                       uintptr
	RegisterControlChangeNotify   uintptr
	UnregisterControlChangeNotify uintptr
	GetChannelCount               uintptr
	SetMasterVolumeLevel          uintptr
	SetMasterVolumeLevelScalar    uintptr
	GetMasterVolumeLevel          uintptr
	GetMasterVolumeLevelScalar    uintptr
	SetChannelVolumeLevel         uintptr
	SetChannelVolumeLevelScalar   uintptr
	GetChannelVolumeLevel         uintptr
	GetChannelVolumeLevelScalar   uintptr
	SetMute                       uintptr
	GetMute                       uintptr
	GetVolumeStepInfo             uintptr
	VolumeStepUp                  uintptr
	VolumeStepDown                uintptr
	QueryHardwareSupport          uintptr
	GetVolumeRange                uintptr
}

var (
	ole32                = syscall.NewLazyDLL("ole32.dll")
	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoUninitialize   = ole32.NewProc("CoUninitialize")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
	coreAudioNeedsUninit bool
	coreAudioUsable      bool

	clsidMMDeviceEnumerator = guid{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator  = guid{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioEndpointVolume = guid{0x5CDF2C82, 0x841E, 0x4546, [8]byte{0x97, 0x22, 0x0C, 0xF7, 0x40, 0x78, 0x22, 0x9A}}
)

func hresultFailed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func hresultError(action string, hr uintptr) error {
	return fmt.Errorf("%s failed: HRESULT 0x%08X", action, uint32(hr))
}

func initCoreAudio() error {
	hr, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded)
	switch uint32(hr) {
	case 0, 1: // S_OK, S_FALSE
		coreAudioNeedsUninit = true
		coreAudioUsable = true
		logEvent("Core Audio COM initialized: HRESULT=0x%08X", uint32(hr))
		return nil
	case rpcEChangedMode:
		// COM was already initialized on this thread in another apartment model.
		// Core Audio can still be used; this call simply must not be balanced.
		coreAudioUsable = true
		logEvent("Core Audio COM already initialized with another apartment model")
		return nil
	default:
		coreAudioUsable = false
		return hresultError("CoInitializeEx", hr)
	}
}

func closeCoreAudio() {
	if coreAudioNeedsUninit {
		procCoUninitialize.Call()
		coreAudioNeedsUninit = false
	}
}

func releaseEnumerator(value *iMMDeviceEnumerator) {
	if value != nil && value.lpVtbl != nil {
		syscall.SyscallN(value.lpVtbl.Release, uintptr(unsafe.Pointer(value)))
	}
}

func releaseDevice(value *iMMDevice) {
	if value != nil && value.lpVtbl != nil {
		syscall.SyscallN(value.lpVtbl.Release, uintptr(unsafe.Pointer(value)))
	}
}

func releaseEndpointVolume(value *iAudioEndpointVolume) {
	if value != nil && value.lpVtbl != nil {
		syscall.SyscallN(value.lpVtbl.Release, uintptr(unsafe.Pointer(value)))
	}
}

func defaultEndpointVolume() (*iAudioEndpointVolume, error) {
	if !coreAudioUsable {
		return nil, fmt.Errorf("Core Audio is unavailable")
	}

	var enumerator *iMMDeviceEnumerator
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)),
		0,
		clsctxAll,
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)),
		uintptr(unsafe.Pointer(&enumerator)),
	)
	if hresultFailed(hr) || enumerator == nil {
		return nil, hresultError("CoCreateInstance(MMDeviceEnumerator)", hr)
	}
	defer releaseEnumerator(enumerator)

	var device *iMMDevice
	hr, _, _ = syscall.SyscallN(
		enumerator.lpVtbl.GetDefaultAudioEndpoint,
		uintptr(unsafe.Pointer(enumerator)),
		eRender,
		eMultimedia,
		uintptr(unsafe.Pointer(&device)),
	)
	if hresultFailed(hr) || device == nil {
		logEvent("GetDefaultAudioEndpoint(multimedia) failed: HRESULT=0x%08X; trying console role", uint32(hr))
		hr, _, _ = syscall.SyscallN(
			enumerator.lpVtbl.GetDefaultAudioEndpoint,
			uintptr(unsafe.Pointer(enumerator)),
			eRender,
			eConsole,
			uintptr(unsafe.Pointer(&device)),
		)
	}
	if hresultFailed(hr) || device == nil {
		return nil, hresultError("GetDefaultAudioEndpoint", hr)
	}
	defer releaseDevice(device)

	var endpoint *iAudioEndpointVolume
	hr, _, _ = syscall.SyscallN(
		device.lpVtbl.Activate,
		uintptr(unsafe.Pointer(device)),
		uintptr(unsafe.Pointer(&iidIAudioEndpointVolume)),
		clsctxAll,
		0,
		uintptr(unsafe.Pointer(&endpoint)),
	)
	if hresultFailed(hr) || endpoint == nil {
		return nil, hresultError("IMMDevice.Activate(IAudioEndpointVolume)", hr)
	}
	return endpoint, nil
}

// ensureWindowsMasterVolumeMax makes the boost percentage relative to the
// actual Windows 100% endpoint level. It uses volume steps instead of a float
// COM parameter so the implementation remains dependency-free and ABI-safe.
func ensureWindowsMasterVolumeMax() error {
	endpoint, err := defaultEndpointVolume()
	if err != nil {
		return err
	}
	defer releaseEndpointVolume(endpoint)

	var currentStep uint32
	var stepCount uint32
	hr, _, _ := syscall.SyscallN(
		endpoint.lpVtbl.GetVolumeStepInfo,
		uintptr(unsafe.Pointer(endpoint)),
		uintptr(unsafe.Pointer(&currentStep)),
		uintptr(unsafe.Pointer(&stepCount)),
	)
	if hresultFailed(hr) {
		return hresultError("IAudioEndpointVolume.GetVolumeStepInfo", hr)
	}
	if stepCount == 0 || currentStep >= stepCount {
		return fmt.Errorf("Windows reported invalid volume step data: current=%d count=%d", currentStep, stepCount)
	}

	stepsNeeded := int(stepCount-currentStep) - 1
	if stepsNeeded > 512 {
		return fmt.Errorf("Windows reported an unreasonable volume step count: current=%d count=%d", currentStep, stepCount)
	}
	for i := 0; i < stepsNeeded; i++ {
		hr, _, _ = syscall.SyscallN(
			endpoint.lpVtbl.VolumeStepUp,
			uintptr(unsafe.Pointer(endpoint)),
			0,
		)
		if hresultFailed(hr) {
			return hresultError("IAudioEndpointVolume.VolumeStepUp", hr)
		}
	}

	var verifiedStep uint32
	var verifiedCount uint32
	hr, _, _ = syscall.SyscallN(
		endpoint.lpVtbl.GetVolumeStepInfo,
		uintptr(unsafe.Pointer(endpoint)),
		uintptr(unsafe.Pointer(&verifiedStep)),
		uintptr(unsafe.Pointer(&verifiedCount)),
	)
	if hresultFailed(hr) {
		return hresultError("IAudioEndpointVolume.GetVolumeStepInfo(verify)", hr)
	}
	if verifiedCount == 0 || verifiedStep+1 != verifiedCount {
		return fmt.Errorf("Windows master volume did not reach 100%%: current=%d count=%d", verifiedStep, verifiedCount)
	}
	logEvent("Windows master volume synchronized to 100%%: previousStep=%d stepCount=%d stepsRaised=%d", currentStep, stepCount, stepsNeeded)
	return nil
}
