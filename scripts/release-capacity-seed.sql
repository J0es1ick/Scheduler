
\set ON_ERROR_STOP on
BEGIN;
DO $$ BEGIN
  IF current_database() NOT LIKE '%\_release_test' ESCAPE '\' THEN
    RAISE EXCEPTION 'capacity fixture requires a dedicated *_release_test database';
  END IF;
  IF EXISTS (SELECT 1 FROM users) THEN
    RAISE EXCEPTION 'capacity fixture requires an empty users table';
  END IF;
END $$;
UPDATE data_sources SET is_enabled=FALSE;
INSERT INTO universities(id,name,timezone) SELECT 'capacity-u-'||i,'Синтетический вуз '||i,'Europe/Moscow' FROM generate_series(0,1) i;
INSERT INTO semesters(id,external_id,university_id,name,start_date,end_date)
SELECT 'capacity-term-'||i,'term','capacity-u-'||i,'Тестовый семестр',CURRENT_DATE-30,CURRENT_DATE+180 FROM generate_series(0,1) i;
INSERT INTO groups(id,university_id,name)
SELECT 'capacity-g-'||i,'capacity-u-'||(i%2),'ТЕСТ-'||i FROM generate_series(0,999) i;
INSERT INTO lessons(id,university_id,semester_id,group_id,day_of_week,time_start,time_end,week_type,subject,type,room,valid_from,valid_to)
SELECT 'capacity-l-'||i,'capacity-u-'||((i/50)%2),'capacity-term-'||((i/50)%2),'capacity-g-'||(i/50),
       (i%5)+1,'09:00','10:30','every','Синтетическое занятие '||i,'lecture','Тестовая аудитория',CURRENT_DATE-30,CURRENT_DATE+180
FROM generate_series(0,49999) i;
INSERT INTO users(id,default_group_id,notifications_enabled)
SELECT (9020000000+i)::text,'capacity-g-'||(i%1000),TRUE FROM generate_series(0,9999) i;
INSERT INTO subscriptions(id,user_id,object_id,object_type)
SELECT 'capacity-sub-'||i||'-'||j,(9020000000+i)::text,'capacity-g-'||((i+j)%1000),'group'
FROM generate_series(0,9999) i CROSS JOIN generate_series(0,1) j;
INSERT INTO bot_outbox(id,user_id,kind,body)
SELECT 'capacity-message-'||i,(9020000000+i)::text,'support_resolution','Синтетическая проверка доставки' FROM generate_series(0,9999) i;
COMMIT;
ANALYZE;
SELECT (SELECT count(*) FROM users) AS users, (SELECT count(*) FROM subscriptions) AS subscriptions,
       (SELECT count(*) FROM groups WHERE id LIKE 'capacity-%') AS groups, (SELECT count(*) FROM lessons WHERE id LIKE 'capacity-%') AS lessons;
