<div align="center">
  <img src="assets/YetAnotherVolumeBooster.png" alt="YetAnotherVolumeBooster icon" width="120" height="120">

# YetAnotherVolumeBooster

### A clean, native Windows volume amplifier with system-wide gain from 100% to 500%.

[![Windows](https://img.shields.io/badge/Windows-10%20%7C%2011-0078D4?logo=windows&logoColor=white)](#requirements)
[![Architecture](https://img.shields.io/badge/Architecture-x64-555555)](#requirements)
[![Version](https://img.shields.io/badge/version-1.7.0-0f7a52)](https://github.com/BlazinSan/YetAnotherVolumeBooster/releases/latest)
[![License](https://img.shields.io/badge/license-MIT-0f7a52)](LICENSE.txt)
[![Equalizer APO](https://img.shields.io/badge/audio%20engine-Equalizer%20APO%201.4.2-189a6c)](https://sourceforge.net/projects/equalizerapo/)

[Download the latest release](https://github.com/BlazinSan/YetAnotherVolumeBooster/releases/latest) · [Report a bug](../../issues/new)
</div>

---



https://github.com/user-attachments/assets/08c96651-e29d-4624-986d-b4f42d5a078e


<img src="https://github.com/user-attachments/assets/5f596644-b936-44a0-8a5f-4dc53aa96949" width="49.5%"></img> <img src="https://github.com/user-attachments/assets/ccba9d26-6b74-4a69-a335-8036a60d849f" width="49.5%"></img>


## About

YetAnotherVolumeBooster is a lightweight Windows utility that increases shared-mode system audio beyond the normal 100% limit. It provides a polished 100–500% gain controller while using [Equalizer APO](https://sourceforge.net/projects/equalizerapo/) as the underlying Windows Audio Processing Object.

It is designed to feel like a small native Windows utility rather than a full equalizer suite: install it once, select your playback device, and control the boost from the main window or system tray.

> [!IMPORTANT]
> YetAnotherVolumeBooster does **not** install its own background service. Equalizer APO runs inside the Windows audio engine. A true app exit resets YetAnotherVolumeBooster's managed gain to **100% / 0.00 dB**. Closing to the tray intentionally keeps the selected boost active.

## Features

- **100–500% system-wide gain** for normal Windows shared-mode audio
- **Smooth precision slider** with direct 100%, 200%, 300%, 400%, and 500% presets
- **Automatic Windows master-volume synchronization** when boost is above 100%
- **Emerald light and dark themes** with a circular theme-switch animation
- **Native anti-aliased interface** with rounded cards, switches, and controls
- **System tray controls** for quickly changing gain without opening the window
- **Start with Windows** option that launches quietly into the tray
- **Close-to-tray** option with a separate true Exit command
- **Live Equalizer APO configuration updates** without restarting the player
- **Device Setup** and **Repair Integration** tools built into the app
- **Diagnostic logs** for controller, installer, APO integration, and device-selector failures
- **Safe exit behavior** that restores 0 dB before terminating
- **No Python, .NET, Electron, or browser runtime required**
- Native **Windows x64** executables built with Go

## Gain levels

YetAnotherVolumeBooster displays a linear amplitude multiplier and converts it to Equalizer APO preamp gain:

| YetAnotherVolumeBooster | APO gain |
|---:|---:|
| 100% | 0.00 dB |
| 200% | +6.02 dB |
| 300% | +9.54 dB |
| 400% | +12.04 dB |
| 500% | +13.98 dB |

> [!CAUTION]
> High gain can clip already-loud audio, damage speakers or headphones, and harm your hearing. Begin at 100%, increase gradually, and lower the level immediately if you hear distortion.

## Requirements

- Windows 10 or Windows 11
- 64-bit Windows installation
- Administrator access during setup
- A playback device that supports Windows audio enhancements/APOs
- Shared-mode audio playback

ASIO and WASAPI exclusive-mode applications may bypass the Windows APO pipeline and therefore may not be affected by YetAnotherVolumeBooster.

## Installation

1. Open the repository's **[Releases](https://github.com/BlazinSan/YetAnotherVolumeBooster/releases/latest)** page.
2. Download `YetAnotherVolumeBoosterSetup.exe` from the latest release.
3. Run the installer and approve the Windows administrator prompt.
4. Confirm the installation when prompted.
5. If Equalizer APO is not already installed, YetAnotherVolumeBooster will:
   - download the official Equalizer APO 1.4.2 x64 installer;
   - verify its SHA-256 checksum before execution; and
   - install it automatically.
6. In the Equalizer APO device selector, select the **actual speakers, headphones, Bluetooth device, USB DAC, or monitor output you use**.
7. Click **OK** in the device selector.
8. Restart Windows if setup says the Windows Audio service could not be restarted automatically.
9. Open YetAnotherVolumeBooster and compare 100% with 200% while audio is playing.

### Windows SmartScreen

Community builds may be unsigned and can trigger a Microsoft Defender SmartScreen warning. Download YetAnotherVolumeBooster only from this repository's Releases page and compare the file checksum with the release notes when one is provided.

## Usage

### Main window

Drag the slider or select one of the preset buttons. Values above 100% set the active Windows playback endpoint to its maximum volume and then apply additional APO gain.

The status card reports the active state, for example:

```text
Active · Windows 100% · APO +6.02 dB
```

### System tray

The tray menu provides:

- Open YetAnotherVolumeBooster
- 100%, 200%, 300%, 400%, and 500% presets
- Start with Windows toggle
- Close button minimizes to tray toggle
- Exit

Left-click or double-click the tray icon to reopen the window.

### Closing and exiting

| Action | Result |
|---|---|
| Click **X** with close-to-tray enabled | Window hides; app and selected gain stay active |
| Click **X** with close-to-tray disabled | Gain resets to 100%; app exits |
| Select **Exit** from the tray menu | Gain resets to 100%; app exits |
| Windows startup enabled | App starts quietly in the tray |

## How it works

```text
YetAnotherVolumeBooster.exe
      │
      ├─ sets the active Windows playback endpoint to 100% when needed
      │
      └─ writes a managed preamp value to:
         C:\Program Files\EqualizerAPO\config\YetAnotherVolumeBooster\gain.txt
                              │
                              └─ Equalizer APO reloads the configuration
                                 and processes shared-mode Windows audio
```

YetAnotherVolumeBooster adds one managed block to Equalizer APO's main configuration:

```text
# BEGIN YetAnotherVolumeBooster
Include: YetAnotherVolumeBooster\gain.txt
# END YetAnotherVolumeBooster
```

Existing Equalizer APO configuration is preserved, and the installer creates a backup before modifying `config.txt`.

## Updating

1. Download the newest `YetAnotherVolumeBoosterSetup.exe`.
2. Run it over the existing installation.
3. Confirm the active playback device if the device selector opens.
4. Restart Windows only if requested.

Your user preferences and managed gain configuration are preserved during a normal update.

## Uninstallation

1. Open **Windows Settings → Apps → Installed apps**.
2. Find **YetAnotherVolumeBooster**.
3. Select **Uninstall** and confirm.

The uninstaller removes YetAnotherVolumeBooster, its shortcuts, startup entry, managed APO include, gain file, and ProgramData diagnostic logs. Equalizer APO is intentionally left installed to avoid damaging unrelated audio configurations. Remove Equalizer APO separately from Windows Settings only when no other application or configuration uses it. To remove saved UI preferences as well, delete `%APPDATA%\YetAnotherVolumeBooster`.

## Troubleshooting

### The percentage changes, but the sound does not

- Open **Device Setup** and confirm that the currently active playback device is selected.
- Make sure Windows is outputting audio through that same device.
- Restart Windows once after selecting a new device.
- Confirm that audio enhancements are not disabled for the playback device.
- Test with a normal shared-mode app such as a browser, media player, or Spotify desktop.
- ASIO and WASAPI exclusive-mode applications can bypass Equalizer APO.
- Use **Repair Integration** to rebuild the managed configuration and reopen the device selector.

### 200% does not sound twice as loud

The displayed percentage is an amplitude multiplier, not a perceived-loudness measurement. Human loudness perception is logarithmic, and source material may already be normalized, limited, or mastered near full scale. Higher gain may create clipping before it produces a clean perceived-volume increase.

### The gain remains active after closing the window

The **Close button minimizes to tray** setting is enabled. Use **Exit** from the tray icon, or disable that setting before clicking X. A true exit resets the managed preamp to 0.00 dB.

### Device Selector reports a Qt platform-plugin error

Run **Repair Integration** from YetAnotherVolumeBooster. The repair launcher isolates Equalizer APO's Qt environment, verifies the platform plugin, and records detailed diagnostics if the selector still fails.

### No tray icon appears

Check the Windows notification-area overflow menu. If YetAnotherVolumeBooster is running but the icon is not visible, end `YetAnotherVolumeBooster.exe` in Task Manager and launch it again from the Start menu.

## Diagnostic logs

Use the **Logs** button in YetAnotherVolumeBooster, or open:

```text
C:\ProgramData\YetAnotherVolumeBooster\logs\YetAnotherVolumeBooster.log
C:\ProgramData\YetAnotherVolumeBooster\logs\YetAnotherVolumeBoosterSetup.log
```

When reporting a bug, attach both files and include:

- Windows version
- Playback-device name
- Whether the device is Bluetooth, USB, HDMI/DisplayPort, or built-in audio
- The YetAnotherVolumeBooster percentage tested
- Whether a Windows restart was performed after device setup

## Building from source

### Prerequisites

- Windows 10/11 x64
- [Go 1.23 or newer](https://go.dev/dl/)
- PowerShell

### Build

```powershell
git clone https://github.com/BlazinSan/YetAnotherVolumeBooster.git
cd YetAnotherVolumeBooster
powershell -ExecutionPolicy Bypass -File .\build.ps1
```

The output is written to:

```text
dist\YetAnotherVolumeBooster.exe
dist\YetAnotherVolumeBoosterSetup.exe
```

### Online installer build

The repository includes a small placeholder at:

```text
setup\payload\EqualizerAPO-x64-1.4.2.exe
```

With the placeholder present, `YetAnotherVolumeBoosterSetup.exe` downloads the official Equalizer APO installer during setup and verifies this expected SHA-256 value:

```text
7403BE7427BBE1936A40DDED082829B6E217FC4F5990FEE5CBA501F0AE055AFA
```

### Fully offline installer build

1. Download the official `EqualizerAPO-x64-1.4.2.exe` installer.
2. Verify that its SHA-256 matches the value above.
3. Place it at `setup\payload\EqualizerAPO-x64-1.4.2.exe`.
4. Run `build.ps1` again.

The setup program will embed the verified installer and produce one offline installer executable.

## Project layout

```text
app/                 Native YetAnotherVolumeBooster controller
setup/               Installer, repair, and uninstall program
setup/payload/       Embedded application and optional APO installer
assets/              Application artwork and icons
licenses/            Third-party licence notices
build.ps1            Windows x64 build script
dist/                 Compiled executables
```

## Privacy and networking

YetAnotherVolumeBooster contains no analytics, accounts, advertisements, or telemetry. The controller does not require internet access. The online setup build connects to SourceForge only when Equalizer APO is missing and its installer must be downloaded. The downloaded file is rejected unless its SHA-256 checksum matches the expected official value.

## License

YetAnotherVolumeBooster's original source code is released under the [MIT License](LICENSE.txt).

Equalizer APO is a separate third-party project distributed under the GNU GPL v2. Its licence and source information are included under `licenses/`; the installer also writes `THIRD-PARTY-NOTICES.txt` into the installation directory.

## Acknowledgements

- [Equalizer APO](https://sourceforge.net/projects/equalizerapo/) — system-wide Windows audio-processing engine

---

<div align="center">
  <strong>Protect your hearing. More gain is not always better gain.</strong>
</div>
