# FreeNews

A tool to end paywalls by using DNS and a reverse proxy.

### How does it work ?

The focus of this tool is to make the actual usage as easy as possible.
This means we do not rely on browsers or extensions like Hover.
The approch this application follows can be split into two different parts:

Reverse Proxy:
We utilize a simple reverse proxy that listens on any domain and just forwards request to the host at which the original domain points. While doing so the proxy injects a bunch of different bypass techniques into your requests. This basicly allows us to duplicate websites to a server that we own and modify them.

DNS Server:
By providing a selfhosted nameserver and a self singed CA we are able to actually "steal" the original domains (at least while you are using that DNS server).
The nameserver just mirrors 1.1.1.1 (cloudflare dns server), but overrides all requests that go to a list of customizable sites to be redirect to our reverse proxy host (basilcy saying replaces the original website with the unpaywalled one).

### How to install ?

1. buy server
2. clone repo
3. install go
4. sudo go run .
5. open udp 53, tcp 80 and 443
6. on you phone set your dns server to your server ip
7. go to freenews.xxx
8. download and install ca file (apps not wifi)

everything should be working, make sure to share your dns server and ca file with your friends so they can profit aswell.

### Page is not supported ?

This project ships with a list of various news sites which is still pretty small, so in case you want to apped the list feel free to open a PR.
