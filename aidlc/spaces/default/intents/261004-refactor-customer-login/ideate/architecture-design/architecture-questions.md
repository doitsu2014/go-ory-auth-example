# Architecture questions

YOLO mode. The main choice (proxy + opaque id) was made by the product owner
in chat. Recommendation recorded: keep `pseudonym` as the primary key and the
Kratos handle, and add `lookup_key`. This means no Kratos migration, no AAD
change, and the courier is untouched.
