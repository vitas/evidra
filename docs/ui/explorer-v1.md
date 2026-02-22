# Evidence Explorer v1

Evidence Explorer is a read-only investigation UI for Change case files.

## Core workflow
1. Select subject and time range.
2. Find changes.
3. Select a Change.
4. Inspect timeline and payload evidence.
5. Export evidence pack.

## Required behavior
- Change list is the navigation surface.
- Selected Change header is the context anchor.
- Timeline is the investigation surface.
- Payload view shows raw structured evidence.
- Search is filter-driven and can use free-text query.

## Non-goals
- Monitoring/alerting dashboard.
- Governance or approval workflow UI.
- Deployment control surface.

## Verification focus
- Empty/loading/error/partial states are explicit.
- Selection updates header and timeline deterministically.
- Correlation and external annotation fields are visible and searchable.
- Export flow is functional from selected Change context.
