# Evidence Explorer UI Use Cases (v0.1)

This file maps investigation use cases to automated Playwright coverage.

| Use case ID | User intent | Expected outcome | Automated test |
| --- | --- | --- | --- |
| `UC-001` | Open the Evidra extension and investigate recent production changes | Changes list loads, a Change can be selected, timeline and payload are visible | `ui/e2e/evidra.spec.ts` → `UC-001 S1 happy path: list changes and open one` |
| `UC-002` | Find a Change by enterprise correlation reference | Search filters results and selected Change shows matching metadata | `ui/e2e/evidra.spec.ts` → `UC-002 S2 search by correlation key` |
| `UC-003` | Understand a no-match query result | UI shows explicit no-results state and does not remain in loading state | `ui/e2e/evidra.spec.ts` → `UC-003 S3 empty-state clarity` |
| `UC-004` | Produce evidence export from selected Change | Export transitions to completed state and download action is available | `ui/e2e/evidra.spec.ts` → `UC-004 S4 export evidence` |
| `UC-005` | Investigate in narrow viewport without layout breakage | No horizontal overflow; controls remain usable; panels remain contained | `ui/e2e/evidra.spec.ts` → `UC-005 S5 responsive containment smoke` |
| `UC-006` | Get incident outcome and root-cause hint immediately | Root Cause Banner shows status, root-cause text, correlators, and key actions | `ui/e2e/evidra.spec.ts` → `UC-006 S6 root cause banner shows immediate incident summary` |
| `UC-007` | Scan change context with reduced cognitive load | Selected Change header separates primary and secondary fields with expandable details | `ui/e2e/evidra.spec.ts` → `UC-007 S7 selected change header uses primary and secondary fields` |
| `UC-008` | Select subjects with long identifiers without layout overflow | Subject selector shows app + namespace/cluster subtitle, maps in-cluster URL, and hides raw URL in visible text | `ui/e2e/evidra.spec.ts` → `UC-008 S8 subject selector shows structured display without raw cluster URL` |

## Maintenance Rule

- Add a new `UC-*` entry here before adding new investigation behavior.
- Keep the same `UC-*` identifier in Playwright test titles.
- Run `bash scripts/check-ui-use-cases.sh` to validate mapping consistency.
