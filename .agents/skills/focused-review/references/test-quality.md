# Lens: Test Quality

A test earns its keep by failing when the behavior breaks. Review tests by
asking: "what bug would this catch?" — a test with no answer is weight, not
safety.

## Hunt

1. **Tests that can't fail** — no assertion; asserting the value just
   constructed; conditions that are tautologically true; Go: forgetting
   `t.Errorf` in a table loop branch.
2. **Mocking the unit under test** — the fake implements the very logic being
   tested, so the test verifies the fake.
3. **Implementation-coupled tests** — asserting internal call order or
   private state instead of observable behavior; these break on refactors and
   pass on bugs.
4. **Missing failure paths** — only the happy path tested: no not-found, no
   unauthorized, no validation-error, no empty-input case for changed
   behavior.
5. **Missing boundary rows** — table tests without empty/nil/zero/limit
   cases where the code branches on them.
6. **Flakiness sources** — sleeps instead of synchronization; time.Now or
   randomness without injection; order-dependent tests sharing mutable
   fixtures or DB rows.
7. **Behavior changed, tests untouched** — a diff that changes logic while
   every existing test passes unmodified deserves suspicion: either coverage
   was missing or the change is untested.

## Do Not Flag

- A single smoke test on trivial glue — minimal is fine, absent is not.
- Table tests intentionally scoped to the changed branch.
- Test helpers/fixtures with mild duplication — clarity beats DRY in tests.

## Grep Starters

```bash
grep -rn 'time.Sleep' backend-go/ --include='*_test.go'
grep -rLn 't.Error\|t.Fatal\|assert' backend-go/ --include='*_test.go'
```
