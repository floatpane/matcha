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
> If you have the [*"other"* Matcha](https://github.com/piqoni/matcha) already installed, you will have to rename the executable to avoid conflicts.

### Manual Binary Download

You can download pre-compiled binaries from the [Releases page](https://github.com/floatpane/matcha/releases).

1. Download the appropriate archive for your architecture (e.g., `matcha_0.17.0_darwin_amd64.tar.gz` or `matcha_0.17.0_darwin_arm64.tar.gz`).
2. Extract the archive.
3. Move the binary to your path:
   ```bash
   mv matcha /usr/local/bin/
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

Currently, there is no native support for Windows. Please see issue [#123](https://github.com/floatpane/matcha/issues/123) for more details.

### WSL

You can run Matcha on Windows using [WSL (Windows Subsystem for Linux)](https://learn.microsoft.com/en-us/windows/wsl/install).

Once you have WSL installed and set up, you can follow the [Linux](#-linux) installation instructions inside your WSL terminal.

## 🏗️ Building from Source

If you have Go installed, you can build Matcha from source:

```bash
go build .
```
