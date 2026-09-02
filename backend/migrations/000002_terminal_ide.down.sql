alter table agent_memberships drop column if exists ide_workspace_id;
alter table agent_memberships drop column if exists host_id;
drop table if exists lsp_servers, ide_collab_sessions, ide_buffers, ide_workspaces, terminal_recordings, terminal_sessions, hosts, ssh_keys, host_groups cascade;
