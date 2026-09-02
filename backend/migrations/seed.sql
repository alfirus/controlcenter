-- seed.sql — dev seed (run with `make migrate:seed DATABASE_URL=...`)
-- idempotent via on conflict

-- storage buckets
insert into storage.buckets (id, name) values
  ('attachments','attachments'),
  ('terminal-recordings','terminal-recordings'),
  ('ide-snapshots','ide-snapshots')
on conflict (id) do nothing;

-- dev auth user (000...001) — instance_id may be null if no supabase instances
insert into auth.users (id, instance_id, aud, role, email, encrypted_password, confirmed_at, raw_app_meta_data, raw_user_meta_data, created_at, updated_at)
values (
  '00000000-0000-0000-0000-000000000001',
  (select id from auth.instances limit 1),
  'authenticated','authenticated','dev@controlcenter.local',
  crypt('password', gen_salt('bf')),
  now(),
  '{"provider":"email","providers":["email"]}',
  '{"display_name":"Dev"}',
  now(), now()
) on conflict (id) do nothing;

insert into profiles (id, display_name) values ('00000000-0000-0000-0000-000000000001','Dev') on conflict (id) do nothing;
insert into workspaces (id, name, created_by) values ('11111111-1111-1111-1111-111111111111','Acme', '00000000-0000-0000-0000-000000000001') on conflict (id) do nothing;
insert into workspace_members (workspace_id, user_id, role) values ('11111111-1111-1111-1111-111111111111','00000000-0000-0000-0000-000000000001','admin') on conflict do nothing;
insert into channels (id, workspace_id, kind, name, purpose, created_by) values
  ('22222222-2222-2222-2222-222222222222','11111111-1111-1111-1111-111111111111','open','general','General discussion','00000000-0000-0000-0000-000000000001'),
  ('33333333-3333-3333-3333-333333333333','11111111-1111-1111-1111-111111111111','open','dev','Dev channel','00000000-0000-0000-0000-000000000001')
on conflict (id) do nothing;
insert into channel_members (channel_id, user_id) values
  ('22222222-2222-2222-2222-222222222222','00000000-0000-0000-0000-000000000001'),
  ('33333333-3333-3333-3333-333333333333','00000000-0000-0000-0000-000000000001')
on conflict do nothing;
insert into messages (channel_id, author_id, body) values
  ('22222222-2222-2222-2222-222222222222','00000000-0000-0000-0000-000000000001','Welcome to Control Center!')
on conflict do nothing;
insert into projects (id, workspace_id, name, description) values ('44444444-4444-4444-4444-444444444444','11111111-1111-1111-1111-111111111111','Control Center','Main project') on conflict (id) do nothing;
insert into tasks (project_id, title, status) values ('44444444-4444-4444-4444-444444444444','Seed task — wire up Supabase Realtime','todo') on conflict do nothing;
insert into host_groups (id, workspace_id, name) values ('55555555-5555-5555-5555-555555555555','11111111-1111-1111-1111-111111111111','Production') on conflict (id) do nothing;
insert into ssh_keys (id, workspace_id, label, vault_ref, fingerprint) values ('66666666-6666-6666-6666-666666666666','11111111-1111-1111-1111-111111111111','prod-key','vault://prod/key','SHA256:abc123') on conflict (id) do nothing;
insert into hosts (id, workspace_id, group_id, label, hostname, port, username, auth_kind, vault_ref, tags) values
  ('77777777-7777-7777-7777-777777777777','11111111-1111-1111-1111-111111111111','55555555-5555-5555-5555-555555555555','prod-01','192.168.1.10',22,'ubuntu','vault_key','vault://prod/key','{prod,web}')
on conflict (id) do nothing;
insert into ide_workspaces (id, workspace_id, project_id, name, root_path) values ('88888888-8888-8888-8888-888888888888','11111111-1111-1111-1111-111111111111','44444444-4444-4444-4444-444444444444','controlcenter','/') on conflict (id) do nothing;
insert into ide_buffers (id, ide_workspace_id, path, content_hash, size_bytes) values ('99999999-9999-9999-9999-999999999999','88888888-8888-8888-8888-888888888888','README.md', encode(sha256('hello'::bytea),'hex'), 5) on conflict (id) do nothing;
insert into agents (id, hermes_endpoint, persona, display_name) values ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa','http://localhost:9000','Sofia','Sofia') on conflict (hermes_endpoint, persona) do nothing;
insert into agent_memberships (agent_id, workspace_id) values ('aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa','11111111-1111-1111-1111-111111111111') on conflict do nothing;
