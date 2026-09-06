# Frontend UI Contract

## Language

- The admin console defaults to Chinese.
- The current account controls include a Chinese / English language switch.
- Backend enum/status values may remain machine-readable until dedicated display labels are mapped.

## Table Controls

- Table-heavy admin pages should use the shared toolbar shape for search and sort controls.
- Client-side search and sorting are acceptable while the dataset is small.
- Future server-side pagination, filtering, and column sorting should reuse the same toolbar shape.

## Recording History

- Recording start time is a first-class table column.
- Displayed recording timestamps use `Asia/Shanghai` and include the visible China Time label.

## Account Management

- Super admins can open an account editor from the accounts table.
- Empty password fields must keep the current password unchanged.
