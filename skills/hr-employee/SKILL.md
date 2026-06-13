# HR Employee

Employee lookup commands:

```bash
hr employee +find --name <name>
hr employee +find --badge <badge>
hr employee +find --phone <phone>
hr employee get --eid <eid>
```

Rules:

- If multiple employees match, stop and ask for a narrower identifier.
- Prefer badge or EID for follow-up operations.
- Output is redacted by the CLI; do not attempt raw queries for sensitive fields unless explicitly needed for diagnostics.

