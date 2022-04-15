# FreeNews

A tool to end client side paywalls.

### How ?

The focus of this tool is to make the actual usage as easy as possible.
This means we do not rely on browsers or extensions like Hover.
The approch this application follows can be split into two different parts:

Reverse Proxy:
We utilize a simple reverse proxy that injects a variety of different bypass techniques into your requests. This basicly allows us to duplicate websites to a domain we own ex: ft.com turns into ft.com.freenews.com.
The idea here is that we dont want to mitm attack your whole browser traffic. By just mirroring websites to our own pc (or server. good if you want to share) we can provide a semi transparent solution. The hoster of the reverse proxy can only hijack the data that goes to the real version of what it its mirroring.

DNS Server:
By providing a selfhosted nameserver we can make the usage of the "domains that we own. mirror thing" even easier. Instead of using a bookmark to redirect you to the unpaywalled version of the page or changing url by hand, you can just set your dns server to the selfhosted one.
The nameserver just mirrors 1.1.1.1 (cloudflare dns server), but overrides all requests that go to a list of customizable sites to be redirect to our reverse proxy host. You may noticed that this actually breaks ssl, because it prevents us from stealing domains. This can be solved by installing a self signed ca file on all your devices. This if far more supported that using browser extensions.

### Tldr; Where download ?

1. buy server
2. clone repo
3. install go
4. go build
5. ./proxy <domain>
6. buy a domain and create a wildcard record to point to the public ip address of your host
7. on you phone set your dns server to your server ip
8. go to freenews.xxx
9. download and install ca file (apps not wifi)

everything should be working, make sure to share your dns server and ca file with your friends so they can profit aswell.

### Page is not supported ?

This project ships with a list of various news sites which is still pretty small, so in case you want to make a new site working. Just go to freenews.xxx on your device and add new domains there.
