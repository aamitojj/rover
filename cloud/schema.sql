-- Apply in a dedicated Supabase project before enabling cloud clients.
begin;
create table public.rover_projects (
 id uuid primary key default gen_random_uuid(),
 owner_id uuid not null default auth.uid() references auth.users(id) on delete cascade,
 name text not null check (length(name) between 1 and 200),
 domain text not null,
 updated_at timestamptz not null default now()
);
alter table public.rover_projects enable row level security;
create policy own_projects on public.rover_projects for all to authenticated
 using (owner_id = auth.uid()) with check (owner_id = auth.uid());
revoke all on public.rover_projects from anon;
grant select, insert, update, delete on public.rover_projects to authenticated;
create table public.rover_daily_analytics (
 project_id uuid not null references public.rover_projects(id) on delete cascade,
 day date not null,
 country text not null default 'Unknown',
 source text not null default 'Unknown',
 path text not null default '/',
 pageviews bigint not null default 0 check (pageviews >= 0),
 primary key (project_id,day,country,source,path)
);
alter table public.rover_daily_analytics enable row level security;
create policy own_analytics on public.rover_daily_analytics for select to authenticated
 using (exists (select 1 from public.rover_projects p where p.id = project_id and p.owner_id = auth.uid()));
revoke all on public.rover_daily_analytics from anon, authenticated;
grant select on public.rover_daily_analytics to authenticated;
-- Only the trusted analytics ingestion backend may write aggregates.
grant select, insert, update, delete on public.rover_daily_analytics to service_role;
commit;
