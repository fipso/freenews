# FreeNews 🔨💵🧱

A paywall bypassing reverse proxy and DNS server written in go.

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

1. buy a vps server
2. install go
3. clone repo
4. go build . && chmod +x freenews
5. ./freenews
6. open udp 53, tcp 80 and 443
7. on you phone set your dns server to your server ip
8. go to free.news
9. download and install ca file (apps not wifi)
10. done. everything should be working, make sure to share your dns server and ca file with your friends so they can profit aswell.

### How do i change the DNS on mobile ?

Android:
Use **one** of the follwing:

1. Wifi Settigns > Use static IP > DNS 1
2. Use a 3rd party app to use DNS or DoT
3. ~~Use private dns option (requires DoT)~~

IOS:
Should be simillar to android.

### How to use DNS over TLS ?

DNS over TLS (DoT) is a new privacy focused way to use normal dns using a tls socket.
To make this work with this project, you have to get yourself a domain and tls cert.
Place the cert file and its private key at `cert/dot_cert.pem` and `cert/dot_key.pem`.
Start freenews with the `-dotDomain <your domain>` flag to enable DoT. Make sure to open port 853/tcp.

For some reason this does currently not work with the private dns option on android.
This is a pitty, because its the only way to use a custom dns outside your wifi without installing third party apps.

### How to add hosts to the unpaywall list ?

You can add new hosts to the list by appending `[host.com]` like blocks to the `config.toml` file.

### TODO

- [ ] Fix DNS over TLS on android
- [ ] Allow tcp connections
- [ ] Improve code quality and comments
- [ ] Provide better usage instructions
- [ ] More config options
- [ ] Make flags overridable by toml config
