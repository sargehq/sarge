# ID Generation in Sarge

This document describes how IDs are generated for different entities in the Sarge orchestrator system.

## Overview

Sarge uses a hierarchical ID system that reflects the 3-tier architecture:
- **Work** → Content-based hash IDs (e.g., `w-8xa`)
- **Tasks** → Hierarchical numbering under works (e.g., `w-8xa.1`, `w-8xa.2`)
- **Beads** → Content-based hash IDs managed by the beads system (e.g., `ac-pjw`)

## Work IDs

Work IDs use content-based hashing similar to beads, ensuring distributed-friendly, deterministic ID generation.

### Generation Algorithm

1. **Content Creation**: Combine branch name, project name, timestamp, and nonce
2. **Hashing**: Generate SHA256 hash of the content
3. **Encoding**: Convert to base36 for compact, readable format
4. **Formatting**: Prefix with `w-` to identify as work ID

### Implementation Details

```go
// GenerateWorkID generates a content-based hash ID for a work
func (db *DB) GenerateWorkID(ctx context.Context, branchName string, projectName string) (string, error) {
    baseLength := 3  // Start with 3 characters for work IDs
    maxAttempts := 30

    for attempt := 0; attempt < maxAttempts; attempt++ {
        // Calculate target length (increases with collisions)
        targetLength := baseLength + (attempt / 10)
        if targetLength > 8 {
            targetLength = 8  // Cap at 8 characters
        }

        // Create content for hashing
        nonce := attempt % 10
        content := fmt.Sprintf("%s:%s:%d:%d",
            branchName,
            projectName,
            time.Now().UnixNano(),
            nonce)

        // Generate SHA256 hash
        hash := sha256.Sum256([]byte(content))

        // Convert to base36 and truncate
        hashStr := toBase36(hash[:])
        if len(hashStr) > targetLength {
            hashStr = hashStr[:targetLength]
        }

        // Create work ID with prefix
        workID := fmt.Sprintf("w-%s", hashStr)

        // Check uniqueness
        if !exists(workID) {
            return workID
        }
    }

    // Fallback to timestamp-based ID
    return fmt.Sprintf("w-%d", time.Now().UnixNano()/1000000), nil
}
```

### Collision Handling

The algorithm handles collisions through:
1. **Nonce rotation**: First 10 attempts use different nonces (0-9)
2. **Length expansion**: After every 10 attempts, increase ID length by 1
3. **Timestamp uniqueness**: Each attempt uses current nanosecond timestamp
4. **Fallback mechanism**: After 30 attempts, use millisecond timestamp

### Examples

```bash
# Creating a work with auto-generated ID
$ sarge work create feature/user-auth
Generated work ID: w-8xa (from branch: feature/user-auth)

$ sarge work create feature/payments
Generated work ID: w-3kp (from branch: feature/payments)
```

## Task IDs

Tasks use hierarchical IDs that show their relationship to works.

### Format

```
<work-id>.<sequence-number>
```

Examples:
- `w-8xa.1` - First task in work w-8xa
- `w-8xa.2` - Second task in work w-8xa
- `w-pay.1` - First task in work w-pay

### Generation Algorithm

```go
// GetNextTaskNumber returns the next available task number for a work
func (db *DB) GetNextTaskNumber(ctx context.Context, workID string) (int, error) {
    // Count existing tasks for this work
    tasks, err := db.GetWorkTasks(ctx, workID)
    if err != nil {
        return 0, err
    }
    return len(tasks) + 1, nil
}

// In plan.go
taskID := fmt.Sprintf("%s.%d", workID, nextNum)
```

### Properties

- **Sequential**: Numbers increment within each work
- **Scoped**: Each work has its own numbering sequence
- **Deterministic**: Tasks are numbered in creation order
- **No gaps**: Numbers are always contiguous (1, 2, 3...)

## Bead IDs

Beads use content-based hash IDs managed by the beads system (beans CLI).

### Format

```
<prefix>-<hash>
```

Prefixes vary by project configuration (e.g., `ac-` for default).

### Algorithm (from beads system)

Similar to work IDs but with:
- Different content components (title, description, metadata)
- Configurable prefix per project
- Longer default length (4 characters)

### Examples

```
ac-pjw  - A bead for implementing a feature
ac-1gt  - A bead for fixing a bug
ac-dzl  - A bead for documentation
```

## ID Hierarchy Example

```
w-8xa                    # Work: Authentication feature
├── w-8xa.1             # Task: Implement login
│   ├── ac-pjw          # Bead: Create login form
│   └── ac-1gt          # Bead: Add validation
└── w-8xa.2             # Task: Implement logout
    └── ac-dzl          # Bead: Add logout button
```

## Benefits of This Design

### Content-Based Work IDs
- **Distributed-friendly**: No central counter needed
- **Meaningful**: Derived from branch name
- **Collision-resistant**: Uses SHA256 with dynamic length
- **Compact**: Base36 encoding keeps IDs short

### Hierarchical Task IDs
- **Clear ownership**: Instantly see which work owns a task
- **Simple ordering**: Natural sequence within work
- **No conflicts**: Scoped to work prevents collisions

### Integration with Beads
- **Consistent patterns**: Similar hash-based approach
- **Clear separation**: Different prefixes prevent confusion
- **Flexible assignment**: Beads can be grouped into tasks as needed

## Database Schema

```sql
-- Works table
CREATE TABLE works (
    id TEXT PRIMARY KEY,           -- e.g., "w-8xa"
    branch_name TEXT,
    worktree_path TEXT,
    -- ...
);

-- Tasks table
CREATE TABLE tasks (
    id TEXT PRIMARY KEY,           -- e.g., "w-8xa.1"
    work_id TEXT,                  -- Foreign key to works
    -- ...
);

-- Task beads junction table
CREATE TABLE task_beads (
    task_id TEXT,                  -- e.g., "w-8xa.1"
    bead_id TEXT,                  -- e.g., "ac-pjw"
    position INTEGER,
    PRIMARY KEY (task_id, bead_id)
);
```

## Command Examples

```bash
# Work creation - always generates unique ID
$ sarge work create feature/auth
Generated work ID: w-8xa (from branch: feature/auth)

$ sarge work create feature/payments
Generated work ID: w-3kp (from branch: feature/payments)

# Planning tasks (auto-generates task IDs)
$ sarge plan --work w-8xa
Created task w-8xa.1 with 2 bead(s): ac-pjw, ac-1gt
Created task w-8xa.2 with 1 bead(s): ac-dzl

# Task execution references hierarchical ID
$ sarge run w-8xa.1
Processing task w-8xa.1...
```

## Implementation Files

- `/internal/db/work.go` - Work ID generation (`GenerateWorkID`, `GetNextTaskNumber`)
- `/internal/db/task.go` - Task ID handling
- `/cmd/work.go` - Work creation command
- `/cmd/plan.go` - Task ID assignment during planning

## Future Considerations

### Potential Enhancements
- Custom prefixes for work IDs (per project)
- Semantic components in task IDs (e.g., `w-8xa.login.1`)
- UUID fallback for extreme collision scenarios

### Compatibility
- IDs are designed to be filesystem-safe (used in directory names)
- Compatible with git branch naming conventions
- URL-safe for use in web interfaces
- Shell-friendly (no special characters requiring escaping)