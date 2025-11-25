create table if not exists teams (
    team_name varchar(255) primary key
);

create table if not exists users (
    user_id varchar(255) primary key,
    username varchar(255) not null,
    team_name varchar(255) not null references teams(team_name),
    is_active boolean not null default true
);

create table if not exists pull_requests (
    pull_request_id varchar(255) primary key,
    pull_request_name varchar(255) not null,
    author_id varchar(255) not null references users(user_id),
    status varchar(20) not null default 'open',
    created_at timestamp with time zone,
    merged_at timestamp with time zone
);

create table if not exists pr_reviewers (
    pull_request_id varchar(255) not null references pull_requests(pull_request_id),
    user_id varchar(255) not null references users(user_id),
    primary key (pull_request_id, user_id)
);
