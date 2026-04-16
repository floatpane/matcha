---
title: Installation Guide
sidebar_position: 2
---

# Installation Guide

Welcome to the installation guide for Matcha.

##  MacOS

### 🍺 Homebrew

The recommended way to install Matcha on macOS is via Homebrew.

```bash
brew tap floatpane/matcha
brew install floatpane/matcha/matcha
```

After installation, run:

```bash
matcha
```

> [!WARNING]
> If you have the [_"other"_ Matcha](https://github.com/piqoni/matcha) already installed, you will have to rename the executable to avoid conflicts.

### Manual Binary Download

You can download pre-compiled binaries from the [Releases page](https://github.com/floatpane/matcha/releases).

1. Download the appropriate archive for your architecture (e.g., `matcha_0.17.0_darwin_amd64.tar.gz` or `matcha_0.17.0_darwin_arm64.tar.gz`).
2. Extract the archive.
3. Move the binary to your path:
   ```bash
   mv matcha /usr/local/bin/
   chmod +x /usr/local/bin/executable_name
   ```
  
4. Run it:
   ```bash
   matcha
   ```

## 🐧 Linux

### 🍺 Homebrew

You can also install Matcha on Linux via Homebrew.

```bash
brew tap floatpane/matcha
brew install floatpane/matcha/matcha
```

### Snap

Matcha is available on the Snap Store.

```bash
sudo snap install matcha
```

### Flatpak

You can install Matcha via Flatpak using the following command:

```bash
flatpak install https://matcha.floatpane.com/matcha.flatpakref
```

### AUR (Arch Linux) (unofficial)

> Note: This is an unofficial package, not maintained by the Matcha team. Use at your own risk. For any issues with the AUR package, please contact the maintainer ([Dominiquini](https://github.com/Dominiquini)).

If you're using Arch Linux, you can install Matcha from the AUR:

```bash
yay -S matcha-client-bin
```

### Nix

You can run Matcha directly using [Nix](https://nixos.org/) flakes:

```bash
nix run github:floatpane/matcha
```

Or install it into your profile:

```bash
nix profile install github:floatpane/matcha
```

### Manual Binary Download

You can download pre-compiled binaries from the [Releases page](https://github.com/floatpane/matcha/releases).

1. Download the appropriate archive for your architecture (e.g., `matcha_0.17.0_linux_amd64.tar.gz` or `matcha_0.17.0_linux_arm64.tar.gz`).
2. Extract the archive.
3. Move the binary to your path:
   ```bash
   mv matcha /usr/local/bin/
   ```
4. Run it:
   ```bash
   matcha
   ```

## 🪟 Windows

### Winget is the recommended way to install Matcha on Windows.

```powershell
winget install --id=floatpane.matcha
```

### Manual Binary Download

You can download pre-compiled binaries from the [Releases page](https://github.com/floatpane/matcha/releases).

1. Move the executable to a permanent, dedicated location on your computer (e.g., C:\CLI Tools\MyTool\).
2. Open the System Properties window by searching for "environment variables" in the Windows Start menu and selecting "Edit the system environment variables".
3. Click the "Environment Variables..." button at the bottom of the System Properties window, under the "Advanced" tab.
4. Locate the "Path" variable in the "User variables for [Your Username]" section (for access only by your user account) or the "System variables" section (for all users).
5. Double-click on the "Path" variable to edit it.
6. Add the path to your executable's folder:

    In the "Edit environment variable" window, click "New".
    Type or paste the full path to the folder where your executable is located (e.g., C:\CLI Tools\MyTool\).
    Click "OK" on all open windows to save the changes.

### WSL

You can run Matcha on Windows using [WSL (Windows Subsystem for Linux)](https://learn.microsoft.com/en-us/windows/wsl/install).

Once you have WSL installed and set up, you can follow the [Linux](#-linux) installation instructions inside your WSL terminal.

## 🏗️ Building from Source

If you have Go installed, you can build Matcha from source:

```bash
go build .
```
