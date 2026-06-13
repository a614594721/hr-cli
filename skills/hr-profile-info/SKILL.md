# HR Profile Info

Profile-info workflow:

```bash
hr profile-info +preview --user-id <user-id> --set field=value
hr profile-info +apply <preview-id> --yes
```

Rules:

- Only whitelisted fields can be changed.
- Sensitive fields require `HR_ADMIN` and `--sensitive` at preview time.
- Apply re-checks authorization, verifies old-value hashes, writes `personal_info`, and verifies direct `EPRE_STAFFREGISTER` trigger sync.
- Preview display is redacted; raw apply values live only in the local ignored preview secret store.

