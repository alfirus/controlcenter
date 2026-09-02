-- 000001_init — core schema (RLS enabled, Supabase Realtime on messages/tasks/events)
-- Apply with: migrate -path backend/migrations -database $DATABASE_URL up

-- extensions
create extension if not exists "pgcrypto";
create extension if not exists "pg_trgm";

-- profiles mirrors auth.users
create table profiles (
  id uuid primary key references auth.users(id) on delete cascade,
  display_name text not null,
  avatar_url text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table workspaces (
  id uuid primary key default gen_random_uuid(),
  name text not null,
  created_by uuid references profiles(id),
  created_at timestamptz not null default now(),
  deleted_at timestamptz
);
create table workspace_members (
  workspace_id uuid references workspaces(id) on delete cascade,
  user_id uuid references profiles(id) on delete cascade,
  role text not null check (role in ('admin','member','guest')),
  created_at timestamptz not null default now(),
  primary key (workspace_id, user_id)
);

create table channels (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  kind text not null check (kind in ('open','private','dm','gm')),
  name text not null,
  purpose text,
  created_by uuid references profiles(id),
  created_at timestamptz not null default now(),
  deleted_at timestamptz
);
create table channel_members (
  channel_id uuid references channels(id) on delete cascade,
  user_id uuid references profiles(id) on delete cascade,
  created_at timestamptz not null default now(),
  primary key (channel_id, user_id)
);
create table channel_permissions (
  channel_id uuid primary key references channels(id) on delete cascade,
  allow_guest_read boolean not null default false
);

create table messages (
  id uuid primary key default gen_random_uuid(),
  channel_id uuid not null references channels(id) on delete cascade,
  author_id uuid references profiles(id),
  agent_id uuid, -- references agents(id) after agents table
  body text not null,
  root_id uuid references messages(id),
  search tsvector generated always as (to_tsvector('english', body)) stored,
  created_at timestamptz not null default now(),
  edited_at timestamptz,
  deleted_at timestamptz
);
create index messages_search_idx on messages using gin(search);
create index messages_channel_created_idx on messages(channel_id, created_at desc, id);

create table reactions (
  message_id uuid references messages(id) on delete cascade,
  user_id uuid references profiles(id) on delete cascade,
  emoji text not null,
  created_at timestamptz not null default now(),
  primary key (message_id, user_id, emoji)
);

create table webhooks (id uuid primary key default gen_random_uuid(), channel_id uuid references channels(id), url text not null, created_at timestamptz default now());
create table slash_commands (id uuid primary key default gen_random_uuid(), workspace_id uuid references workspaces(id), trigger text not null, url text not null);
create table bots (id uuid primary key default gen_random_uuid(), workspace_id uuid references workspaces(id), name text not null, token text not null unique);

create table projects (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid not null references workspaces(id) on delete cascade,
  name text not null,
  description text,
  created_at timestamptz not null default now(),
  deleted_at timestamptz
);
create table tasks (
  id uuid primary key default gen_random_uuid(),
  project_id uuid not null references projects(id) on delete cascade,
  title text not null,
  body text,
  status text not null default 'todo' check (status in ('todo','doing','done')),
  assignee_id uuid references profiles(id),
  github_issue_id bigint,
  github_repo text,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now(),
  deleted_at timestamptz
);
create unique index tasks_github_unique on tasks(github_repo, github_issue_id) where github_issue_id is not null;
create table task_comments (id uuid primary key default gen_random_uuid(), task_id uuid references tasks(id) on delete cascade, author_id uuid references profiles(id), body text not null, created_at timestamptz default now());

create table github_installations (installation_id bigint primary key, account_login text not null, created_at timestamptz default now());
create table github_repo_links (workspace_id uuid references workspaces(id), installation_id bigint references github_installations(installation_id), repo text not null, project_id uuid references projects(id), primary key (workspace_id, repo));

create table calendars (
  id uuid primary key default gen_random_uuid(),
  workspace_id uuid references workspaces(id) on delete cascade,
  owner_id uuid references profiles(id),
  kind text not null check (kind in ('team','personal')),
  google_calendar_id text,
  name text not null,
  created_at timestamptz default now()
);
create table events (
  id uuid primary key default gen_random_uuid(),
  calendar_id uuid not null references calendars(id) on delete cascade,
  title text not null,
  description text,
  start_at timestamptz not null,
  end_at timestamptz not null,
  google_event_id text,
  etag text,
  created_at timestamptz default now(),
  updated_at timestamptz default now(),
  deleted_at timestamptz
);
create unique index events_google_unique on events(calendar_id, google_event_id) where google_event_id is not null;

create table agents (
  id uuid primary key default gen_random_uuid(),
  hermes_endpoint text not null,
  persona text not null,
  display_name text not null,
  capabilities jsonb not null default '{}',
  created_at timestamptz default now(),
  unique (hermes_endpoint, persona)
);
alter table messages add constraint messages_agent_fk foreign key (agent_id) references agents(id);
create table agent_memberships (
  agent_id uuid references agents(id) on delete cascade,
  workspace_id uuid not null references workspaces(id) on delete cascade,
  channel_id uuid references channels(id) on delete cascade,
  project_id uuid references projects(id) on delete cascade,
  role text not null default 'agent' check (role = 'agent'),
  created_at timestamptz default now(),
  primary key (agent_id, workspace_id, coalesce(channel_id,'00000000-0000-0000-0000-000000000000'), coalesce(project_id,'00000000-0000-0000-0000-000000000000'))
);

create table invites (token text primary key, workspace_id uuid references workspaces(id), role text not null, expires_at timestamptz not null, created_at timestamptz default now());
create table oauth_states (state text primary key, user_id uuid references profiles(id), provider text not null, created_at timestamptz default now());
create table webhook_dead_letters (id uuid primary key default gen_random_uuid(), source text not null, payload jsonb not null, error text, created_at timestamptz default now());
create table audit_log (id uuid primary key default gen_random_uuid(), actor_id uuid references profiles(id), action text not null, target text, created_at timestamptz default now());

-- RLS: enable (policies added in Phase 1; enable now so no table is left open)
alter table profiles enable row level security;
alter table workspaces enable row level security;
alter table workspace_members enable row level security;
alter table channels enable row level security;
alter table channel_members enable row level security;
alter table messages enable row level security;
alter table reactions enable row level security;
alter table projects enable row level security;
alter table tasks enable row level security;
alter table calendars enable row level security;
alter table events enable row level security;
alter table agents enable row level security;
alter table agent_memberships enable row level security;
