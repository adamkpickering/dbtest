db.getSiblingDB("$external").runCommand({
  createUser: "CN=teleport-admin",
  roles: [
    {
      role: "root",
      db: "admin"
    }
  ]
});

db = db.getSiblingDB("test");
db.getSiblingDB("test").runCommand({
  createRole: "creator",
  privileges: [
    {
      resource: { anyResource: true },
      actions: [ "anyAction" ]
    }
  ]
})

db.createCollection("example_table");
db.example_table.insertOne({
  name: "sample",
  description: "Initial test document",
  created_at: new Date()
});
