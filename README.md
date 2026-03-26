# WhatsApp Notifier

A lightweight, background **Go** daemon for **Linux** that triggers system notifications for specific WhatsApp contacts. This project uses the `whatsmeow` library to interface with the WhatsApp multi-device protocol without requiring a browser tab or the official desktop client.

---

## Features

* **Zero-UI Operation**: Runs entirely in the background.
* **Targeted Notifications**: Filter incoming messages for a specific **VIP contact**.
* **Lightweight**: Consumes significantly less RAM than Electron apps.
* **Linux Native**: Uses `notify-send` for desktop integration.
* **Persistent Session**: Authenticate once; session keys are stored in a local SQLite database.

---

## Prerequisites

Ensure you have the following installed on your Linux system:

* **Go** (1.20 or later)
* **GCC / Build Essentials** (Required for SQLite drivers)
* **libnotify** (Provides the `notify-send` command)
* **Bash** (To run the build script)

---

## Installation & Build

1.  **Clone the repository**:
    ```bash
    git clone https://github.com/marcel-valdez/whatsapp-notification.git
    cd whatsapp-notification
    ```

2.  **Run the Build Script**:
    This script handles module initialization, dependency downloads, and compilation.
    ```bash
    chmod +x build.sh
    ./build.sh
    ```

The compiled binary will be located in the `bin/` directory.

---

## Usage

### 1. Initial Setup (Authentication)
Run the program for the first time to generate the authentication QR code. You **must** provide a target JID (even if you're just looking for one).

```bash
./bin/whatsapp-notification -target 1234567890@s.whatsapp.net
```

* **Scan the QR code**: Open WhatsApp on your phone > Linked Devices > Link a Device.
* **Session Persistence**: Once authenticated, `whatsapp-notification.db` is created. You won't need to scan again unless you log out from your phone.

### 2. Identifying a JID

If you don't know your contact's internal ID, run the script and wait for them to message you. The terminal will output:
`Ignoring: 1234567890@s.whatsapp.net`
Copy that ID and restart the script with it as the `-target`.

Note that you may specify multiple comma-separated JIDs.

---

## Running as a Background Daemon

To keep the Whatsapp Notifier running after you close your terminal, create a **systemd** user service at `~/.config/systemd/user/whatsapp-notification.service`:

```ini
[Unit]
Description=WhatsApp Notifier

[Service]
# You will need the SQLite DB to be under the working directory.
WorkingDirectory=/home/<username>/.config/whatsapp-notification/
ExecStart=/home/<username>/bin/whatsapp-notification -target 1234567890@s.whatsapp.net
Restart=always

[Install]
WantedBy=default.target
```

Enable and start it:
```shell
systemctl --user enable --now whatsapp-notification.service
```

Read the service's logs:

```shell
journalctl --user -u whatsapp-notification.service -f
```

---

## License

[MIT](https://choosealicense.com/licenses/mit/)
