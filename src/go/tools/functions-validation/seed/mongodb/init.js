db = db.getSiblingDB("netdata")
db.setProfilingLevel(2)

db.sample.insertMany([
  { name: "alpha", value: 10 },
  { name: "beta", value: 20 },
  { name: "gamma", value: 30 }
])

db.sample.find({ value: { $gt: 15 } }).toArray()
db.sample.updateOne({ name: "alpha" }, { $inc: { value: 1 } })
