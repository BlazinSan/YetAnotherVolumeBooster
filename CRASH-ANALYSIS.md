# Runtime notes

YetAnotherVolumeBooster does not use a background service. Equalizer APO is loaded by the Windows audio engine, so gain remains active after the controller closes unless the configuration is reset. Version 1.7.0 explicitly writes `Preamp: 0.00 dB` before a real exit. Closing to the tray intentionally keeps the selected gain active.
