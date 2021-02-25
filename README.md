# Edge Pinger

This package provides simple APIs for pinging services in a variety of applications.

Multiple drivers and middlewares are included to make it easy to build pinger applications with predictable and manageable behaviour.

## All Drivers and Middleware

| Type | Driver | Description |
|:-----|:-------|:------------|
| Driver | [Dummy()](./dummy.go) | Dummy driver that doesn't connect out. Useful for tests |
| Middleware | [Errors()](./error.go) | Cause pinger to randomly (or always) fail. Useful for tests |
| Driver | [HTTP()](./http.go) | Simple HTTP-based pinger using GET or HEAD |
| Driver | [ICMP()](./icmp.go) | ICMP pinger. Requires root privileges |
| Middleware | [Log()](./log.go) | Logger |
| Middleware | [Track()](./stats.go) | Track ping statistics |
