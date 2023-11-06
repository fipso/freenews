# FreeNews 🔨💵🧱

![build status](https://github.com/fipso/freenews/actions/workflows/go.yml/badge.svg?branch=main)

Reverse Proxy & DNS based solution to bypass paywalls written in go

### Features

- Pull from Google Cache Bypass (shoutout to 12ft.io)
- HTTP Header based bypasses
    - AdsBot-Google User Agent
    - X-Forwarded-For Google Datacenter IP
    - Twitter t.co Referer
    - Drop Cookie & Set-Cookie
- HTTP Body patches
    - Disable JS. Removes <script> tags
    - Inject custom html/js
- DNS/Hosts based AdBlock

### Usage

1. Tell your devices to use your own DNS server
2. Go to `free.news`
3. Download and install CA file (apps not Wi-Fi)
4. Profit

### How?
- ./freenews spawns a DNS and a HTTP server (and TLS versions of it ofc)
- You install a custom CA on your device
- Your device sends DNS querys to your own DNS. If the site is on your bypass whitelist the server will respond with its own public IP, otherwise it will forward to an upstream DNS like 1.1.1.1
- Your phone then connects to the HTTP reverse proxy mirror that you own
- Freenews HTTP server returns the unpaywalled version

### How to install ?

- Host should have port 53/UDP, 80,443,853/UDP open (DNS ports can be changed)
- If port 53 is blocked try to disable your local DNS server ex: `systemctl stop systemd-resolved`

### Docker

Requirements:

- Docker & docker-compose

1. `mkdir freenews && cd freenews`
2. Get our docker-compose  
   `curl -O https://raw.githubusercontent.com/fipso/freenews/main/docker-compose.yml`
3. Run it `sudo docker-compose up -d`
  
-  Check logs `sudo docker-compose logs --follow`
-  Update `sudo docker-compose pull && sudo docker-compose up -d`
-  Add hosts: edit config.toml and `sudo docker-compose restart`

### Build it yourself

Requirements:

- Go 1.18+
- Currently only Linux is tested (Windows, macOS, etc... should work)

1. `git clone https://github.com/fipso/freenews.git`
2. `cd freenews`
3. `go build . && chmod +x freenews`
4. `sudo setcap CAP_NET_BIND_SERVICE=+eip freenews` (Optional. Allows binding low ports as normal user.)
5. `./freenews`

**Auto Start (systemd)**:  
If you choose docker you obviously dont need this.  
Create `freenews.service` at `/lib/systemd/system/`.  
Example Service:  
```systemd
[Unit]
Description=FreeNews DNS & Reverse Proxy

[Service]
User=<some non root user>
WorkingDirectory=/home/<user>/...
ExecStart=/home/<user>/.../freenews
# DoT & AdBlock example:
#ExecStart=/home/<user>/.../freenews -dotDomain <your domain> -blockList <blocklist file>
Restart=always

[Install]
WantedBy=multi-user.target
```

### How to use DNS over TLS ?

DNS over TLS (DoT) is a new privacy focused way to use normal DNS using a TLS socket.
To make this work with this project, you have to get yourself a domain and SSL cert.
Place the cert (**Copy `fullchain.pem` instead of `cert.pem` to `dot_cert.pem` if you are using Let's Encrypt**) file and its private key at `cert/dot_cert.pem` and `cert/dot_key.pem`.
Start freenews with the `-dotDomain <your domain>` flag to enable DoT. Make sure to open port 853/UDP.

### How do I change the DNS on mobile ?

Android:
Use **one** of the following:

- Recommended: Use private DNS option (requires DoT)
- Wi-Fi Settings > Use static IP > DNS 1
- Use a 3rd party app to use DNS or DoT

IOS:

- Recommended: Generate a DNS [profile](https://dns.notjakob.com/index.html) (requires DoT)

### How to add hosts to the unpaywall list ?

You can add new hosts to the list by appending a `[[host]]` block to the `config.toml` file.

### How do I enable AdBlock ?

1. Download a DNS blocklist  
   ex: `curl -O https://raw.githubusercontent.com/hagezi/dns-blocklists/main/domains/light.txt`
2. Start with `-blockList` param  
   ex: `./freenews -blockList light.txt`

We currently redirect all blocked domains to 127.0.0.1

### TODO

- [x] Fix DNS over TLS
- [x] Add docker image & instructions
- [x] Add DNS based AdBlock
- [x] Add non root running instructions
- [ ] Allow TCP connections
- [ ] Improve code quality and comments
- [ ] Provide better usage instructions
- [ ] More config options
- [ ] Make flags overridable by TOML config

### Credits

- https://github.com/drk1wi/Modlishka Request body compression

### Star History
[![Star History Chart](https://api.star-history.com/svg?repos=fipso/freenews&type=Date)](https://star-history.com/#fipso/freenews&Date)
