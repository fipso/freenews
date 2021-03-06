# FreeNews 🔨💵🧱

A paywall bypassing reverse proxy and DNS server written in go.

**This project is still hard work in progress. Expect stuff to just not work. Setup requires some linux and network knowledge.**

### Why ?

The goal of this project is to provide a unpaywalling solution that works on platforms where modifying your browsers content is not possible.
Basilcy this is something like [Hover](https://github.com/nathan-149/hover-paywalls-browser-extension), but as a reverse proxy (+DNS for better usability). This makes usage on mobile devices really enjoyable.
It can be used on all devices where the user is able to change the DNS and install a self signed CA.

### How does it work ?

The approch this application follows can be split into two different parts:

Reverse Proxy:
We utilize a simple reverse proxy that listens on any domain and just forwards request to the host at which the original domain points. While doing so the proxy injects [a bunch of different bypass techniques](https://medium.datadriveninvestor.com/how-to-bypass-any-paywall-for-free-df87832cbff7) into your requests. This basicly allows us to duplicate websites to a server that we own and modify them.

DNS Server:
By providing a selfhosted nameserver and a self singed CA we are able to actually "steal" the original domains (at least while you are using that DNS server).
The nameserver just mirrors 1.1.1.1 (cloudflare dns server), but overrides all requests that go to a list of customizable sites to be redirect to our reverse proxy host (basilcy saying replaces the original website with the unpaywalled one).

### How to install ?

Requirements:

- Go 1.18+
- Currently only Linux is tested (Windows, MacOS, etc... should work)
- Host should have port 53/udp, 80,443,853/tcp open (dns ports can be changed)
- If port 53 is blocked try to disable your local dns server ex: `systemctl stop systemd-resolved`

1. `git clone https://github.com/fipso/freenews.git`
2. `cd freenews`
3. `go build . && chmod +x freenews`
4. `sudo ./freenews`
5. on you phone set your dns server to your public host ip
6. go to free.news
7. download and install ca file (apps not wifi)

### How do i change the DNS on mobile ?

Android:
Use **one** of the follwing:

- Recommended: Use private dns option (requires DoT)
- Wifi Settigns > Use static IP > DNS 1
- Use a 3rd party app to use DNS or DoT

IOS:

- Recommended: Generate a DNS [profile](https://dns.notjakob.com/index.html) (requires DoT)

### How to use DNS over TLS ?

DNS over TLS (DoT) is a new privacy focused way to use normal dns using a tls socket.
To make this work with this project, you have to get yourself a domain and tls cert.
Place the cert (**Copy fullchain.pem instead of cert.pem to `dot_cert.pem` if you are using Let's Encrypt**) file and its private key at `cert/dot_cert.pem` and `cert/dot_key.pem`.
Start freenews with the `-dotDomain <your domain>` flag to enable DoT. Make sure to open port 853/tcp.

### How to add hosts to the unpaywall list ?

You can add new hosts to the list by appending a `[[host]]` block to the `config.toml` file.

### TODO

- [x] Fix DNS over TLS
- [ ] Allow tcp connections
- [ ] Improve code quality and comments
- [ ] Provide better usage instructions
- [ ] More config options
- [ ] Make flags overridable by toml config
