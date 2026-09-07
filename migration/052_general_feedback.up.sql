ALTER TABLE support_requests DROP CONSTRAINT support_requests_request_type_check;
ALTER TABLE support_requests ADD CONSTRAINT support_requests_request_type_check
    CHECK (request_type IN ('update_existing', 'new_institution', 'feedback'));
