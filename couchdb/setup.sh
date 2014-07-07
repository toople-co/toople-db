#!/bin/sh

# Delete database (useful to start fresh)
# curl -X DELETE http://test:test@127.0.0.1:5984/toople

# Create database
curl -X PUT http://test:test@127.0.0.1:5984/toople

# Add design document
curl -X PUT http://test:test@127.0.0.1:5984/toople/_design/toople -H "Content-type: application/json" -d @design.json

# Add sample data
curl -X POST http://test:test@127.0.0.1:5984/toople/_bulk_docs -H "Content-type: application/json" -d @sampledata.json

# Add `db` user to users database
curl -X POST http://test:test@127.0.0.1:5984/_users -H "Content-type: application/json" -d @user.json

# Add security document
curl -X PUT http://test:test@127.0.0.1:5984/toople/_security -H "Content-type: application/json" -d @security.json
