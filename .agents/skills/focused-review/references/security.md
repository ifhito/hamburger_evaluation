# Lens: Security

Assume every request is hostile and every identifier is someone else's. The
question per endpoint is: who can call this, with what data, and what can they
reach that isn't theirs?

## Hunt

1. **Missing/wrong authorization** — new endpoint without an authz check;
   IDOR: acting on `params.id` without verifying ownership; authz decided
   from client-supplied fields (e.g. an `admin` flag in the body).
2. **Injection** — SQL built by string concatenation/`fmt.Sprintf` instead of
   sqlc parameters; user input in shell commands or file paths
   (`filepath.Clean` + base-dir check for anything path-like).
3. **Input trust at the boundary** — handlers passing unvalidated input
   inward; missing length/range checks on user-scaled fields; mass
   assignment: decoding request JSON straight into a persistence struct so
   extra fields (role, status) sneak through.
4. **JWT pitfalls** — algorithm not pinned on verify; missing expiry check;
   secrets from code instead of env; tokens or credentials written to logs.
5. **Secrets and PII in output** — error responses echoing internals (SQL,
   paths, stack traces); serializers exposing fields the endpoint shouldn't
   (email, password hash) — check what the response struct includes, not what
   the client renders.
6. **Frontend** — user content into `dangerouslySetInnerHTML`; tokens in URLs;
   auth state trusted for authorization (UI hiding is not access control).

## Do Not Flag

- Internal tooling explicitly out of the request path.
- Defense-in-depth suggestions on already-safe code — Suggestion, not
  Critical.
- Missing rate-limiting/CSRF where the platform layer handles it — verify
  before flagging.

## Grep Starters

```bash
grep -rn 'Sprintf.*SELECT\|Sprintf.*INSERT\|Sprintf.*UPDATE\|Sprintf.*DELETE' backend-go/
grep -rn 'dangerouslySetInnerHTML' frontend/src/
```
