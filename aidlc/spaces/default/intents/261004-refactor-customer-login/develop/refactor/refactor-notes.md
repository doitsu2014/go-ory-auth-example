# Refactor notes

No separate refactor pass. The change already removed dead code: the resolve endpoint, `Purpose`, the device pseudonym cache, the resolver client, the Kratos registration and recovery-start client methods, and `LoginTarget`. Nothing else was found to simplify without widening the scope.
