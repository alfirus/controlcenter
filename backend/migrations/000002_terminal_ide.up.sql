-- 000002_terminal_ide — Termius-grade terminal + Zed-grade IDE
-- Mirrors docs/BLUEPRINT.md:3.1, 3.5-3.7

-- hosts & groups (Termius parity)
create table host_groups (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  name text not null,
  parent_id uuid references host_groups(id) on delete set null,
  created_at timestamptz not null default now(),
  unique (workspace_id, name)
);

create table ssh_keys (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  label text not null,
  vault_ref text not null, -- Supabase Vault ref or external vault path; never raw key
  fingerprint text,
  created_by uuid references profiles(id),
  created_at timestamptz not null default now()
);

create table hosts (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  group_id uuid references host_groups(id) on delete set null,
  label text not null,
  hostname text not null,
  port int not null default 22 check (port between 1 and 65535),
  username text not null,
  auth_kind text not null default 'vault_key' check (auth_kind in ('vault_key','vault_password','agent_forward')),
  vault_ref text, -- nullable when agent_forward
  jump_host_id uuid references hosts(id) on delete set null,
  tags text[] not null default '{}',
  created_by uuid references profiles(id),
  created_at timestamptz not null default now(),
  deleted_at timestamptz
);
create index hosts_workspace_idx on hosts(workspace_id) where deleted_at is null;
create index hosts_tags_idx on hosts using gin(tags);

-- terminal sessions & recordings (gateway PTY)
create table terminal_sessions (
  id uuid primary key default gen_random_uuid(),
  host_id uuid not null references hosts(id) on delete cascade,
  user_id uuid references profiles(id) on delete set null,
  agent_id uuid references agents(id) on delete set null,
  status text not null default 'active' check (status in ('active','closed','expired')),
  rows int not null default 24,
  cols int not null default 80,
  started_at timestamptz not null default now(),
  ended_at timestamptz,
  recording_ref text, -- Storage bucket path terminal-recordings/<id>
  check (user_id is not null or agent_id is not null)
);
create index terminal_sessions_host_idx on terminal_sessions(host_id, started_at desc);

create table terminal_recordings (
  id uuid primary key default gen_random_uuid(),
  session_id uuid not null references terminal_sessions(id) on delete cascade,
  chunk_seq int not null,
  data bytea not null,
  created_at timestamptz not null default now(),
  unique (session_id, chunk_seq)
);

-- IDE (Zed parity)
create table ide_workspaces (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  project_id uuid references projects(id) on delete set null,
  host_id uuid references hosts(id) on delete set null, -- remote root (terminal integration)
  name text not null,
  root_path text not null default '/',
  created_by uuid references profiles(id),
  created_at timestamptz not null default now(),
  deleted_at timestamptz
);
create index ide_workspaces_ws_idx on ide_workspaces(workspace_id) where deleted_at is null;

create table ide_buffers (
  id uuid primary key default gen_random_uuid(),
  ide_workspace_id uuid not null references ide_workspaces(id) on delete cascade,
  path text not null,
  content_hash text not null, -- sha256 hex
  size_bytes int not null default 0,
  updated_at timestamptz not null default now(),
  unique (ide_workspace_id, path)
);

-- raw snapshot storage is via Storage bucket ide-snapshots; table tracks metadata only

create table ide_collab_sessions (
  id uuid primary key default gen_random_uuid(),
  buffer_id uuid not null references ide_buffers(id) on delete cascade,
  workspace_id uuid not null references workspaces(id) on delete cascade,
  crdt_state jsonb not null default '{}',
  created_at timestamptz not null default now()
);

create table lsp_servers (
  id uuid primary key default gen_random_uuid(),
  language text not null,
  command text not null, -- e.g. gopls, rust-analyzer
  config jsonb not null default '{}',
  created_at timestamptz not null default now(),
  unique (language)
);

-- extend agent_memberships to allow host/ide scoping (nullable new cols)
alter table agent_memberships add column if not exists host_id uuid references hosts(id) on delete cascade;
alter table agent_memberships add column if not exists ide_workspace_id uuid references ide_workspaces(id) on delete cascade;

-- RLS enable on new tables
alter table host_groups enable row level security;
alter table ssh_keys enable row level security;
alter table hosts enable row level security;
alter table terminal_sessions enable row level security;
alter table terminal_recordings enable row level security;
alter table ide_workspaces enable row level security;
alter table ide_buffers enable row level security;
alter table ide_collab_sessions enable row level security;
alter table lsp_servers enable row level security;

-- seed default LSP servers
insert into lsp_servers(language, command) values
  ('go','gopls'),
  ('rust','rust-analyzer'),
  ('typescript','typescript-language-server --stdio'),
  ('python','pylsp')
on conflict (language) do nothing;
