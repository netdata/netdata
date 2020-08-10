// this is the mymodule definition
var crypto = require("crypto");
var https = require("https");
var netdata = require("netdata");
var url = require("url");

var bigbluebutton = {
  charts: {},

  base_priority: 60000,

  processResponse: function(service, data) {
    /* send information to the Netdata server here */
    if (data !== null) {
      if (service.added !== true) {
        service.commit();
      }

      var conferences = (data.match(/<meetingID>/g) || []).length;

      var id = "bbb_" + service.name + ".conferences";
      var chart = bigbluebutton.charts[id];

      if (typeof chart === "undefined") {
        chart = {
          id: "bbb_conferences", // the unique id of the chart
          name: "BBB conferences", // the unique name of the chart
          title: "BBB Current conferences", // the title of the chart
          units: "conferences", // the units of the chart dimensions
          family: "conferences", // the family of the chart
          context: "bigbluebutton.conferences", // the context of the chart
          type: netdata.chartTypes.line, // the type of the chart
          priority: bigbluebutton.base_priority + 1, // the priority relative to others in the same family
          update_every: service.update_every, // the expected update frequency of the chart
          dimensions: {
            conferences: {
              id: "conferences", // the unique id of the dimension
              name: "", // the name of the dimension
              algorithm: netdata.chartAlgorithms.absolute, // the id of the netdata algorithm
              multiplier: 1, // the multiplier
              divisor: 1, // the divisor
              hidden: false // is hidden (boolean)
            }
          }
        };
        chart = service.chart(id, chart);
        bigbluebutton.charts[id] = chart;
      }

      service.begin(chart);
      service.set("conferences", conferences);
      service.end();

      var sumXmlAttr = function(name) {
        var sum = 0;
        var r = new RegExp("<" + name + ">([0-9]+)</" + name + ">", "g");
        var match = r.exec(data);
        while (match !== null) {
          sum += parseInt(match[1], 10);
          match = r.exec(data);
        }
        return sum;
      };

      var participantCount = sumXmlAttr("participantCount");
      var listenerCount = sumXmlAttr("listenerCount");
      var voiceParticipantCount = sumXmlAttr("voiceParticipantCount");
      var videoCount = sumXmlAttr("videoCount");

      id = "bbb_" + service.name + ".users";
      chart = bigbluebutton.charts[id];

      if (typeof chart === "undefined") {
        chart = {
          id: "bbb_users", // the unique id of the chart
          name: "BBB users", // the unique name of the chart
          title: "BBB Current users", // the title of the chart
          units: "users", // the units of the chart dimensions
          family: "users", // the family of the chart
          context: "bigbluebutton.users", // the context of the chart
          type: netdata.chartTypes.line, // the type of the chart
          priority: bigbluebutton.base_priority + 1, // the priority relative to others in the same family
          update_every: service.update_every, // the expected update frequency of the chart
          dimensions: {
            participants: {
              id: "participants", // the unique id of the dimension
              name: "", // the name of the dimension
              algorithm: netdata.chartAlgorithms.absolute, // the id of the netdata algorithm
              multiplier: 1, // the multiplier
              divisor: 1, // the divisor
              hidden: false // is hidden (boolean)
            },
            listeners: {
              id: "listeners", // the unique id of the dimension
              name: "", // the name of the dimension
              algorithm: netdata.chartAlgorithms.absolute, // the id of the netdata algorithm
              multiplier: 1, // the multiplier
              divisor: 1, // the divisor
              hidden: false // is hidden (boolean)
            },
            voiceParticipants: {
              id: "voiceParticipants", // the unique id of the dimension
              name: "", // the name of the dimension
              algorithm: netdata.chartAlgorithms.absolute, // the id of the netdata algorithm
              multiplier: 1, // the multiplier
              divisor: 1, // the divisor
              hidden: false // is hidden (boolean)
            },
            videos: {
              id: "videos", // the unique id of the dimension
              name: "", // the name of the dimension
              algorithm: netdata.chartAlgorithms.absolute, // the id of the netdata algorithm
              multiplier: 1, // the multiplier
              divisor: 1, // the divisor
              hidden: false // is hidden (boolean)
            }
          }
        };
        chart = service.chart(id, chart);
        bigbluebutton.charts[id] = chart;
      }

      service.begin(chart);
      service.set("participants", participantCount);
      service.set("listeners", listenerCount);
      service.set("voiceParticipants", voiceParticipantCount);
      service.set("videos", videoCount);
      service.end();
    }
  },

  configure: function(config) {
    var eligible_services = 0;

    if (typeof config.servers !== "undefined" || config.servers.length !== 0) {
      var len = config.servers.length;
      while (len--) {
        var server = config.servers[len];

        // See https://docs.bigbluebutton.org/dev/api.html#usage
        var checksum = crypto
          .createHash("sha1")
          .update("getMeetings" + server.secret)
          .digest("hex");
        var serverUrl = server.url + "api/getMeetings?checksum=" + checksum;
        var u = url.parse(serverUrl);

        netdata
          .service({
            name: server.name,
            update_every: server.update_every,
            module: this,
            request: {
              protocol: u.protocol,
              hostname: u.hostname,
              port: u.port,
              path: u.path,
              //family: 4,
              method: u.method,
              headers: {
                Connection: "keep-alive"
              },
              agent: new https.Agent({
                keepAlive: true,
                keepAliveMsecs: netdata.options.update_every * 1000,
                maxSockets: 2, // it must be 2 to work
                maxFreeSockets: 1
              })
            }
          })
          .execute(this.processResponse);

        eligible_services++;
      }
    }

    return eligible_services;
  },

  update: function(service, callback) {
    /*
     * this function is called when each service
     * created by the configure function, needs to
     * collect updated values.
     *
     * You normally will not need to change it.
     */

    service.execute(function(service, data) {
      bigbluebutton.processResponse(service, data);
      callback();
    });
  }
};

module.exports = bigbluebutton;
