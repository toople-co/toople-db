function(doc) {
	if (doc.type == 'event') {
		emit([doc._id, doc.type], null);
		for (var i in doc.circles) {
			emit([doc._id, 'circle'], {_id: doc.circles[i]});
		}
	}
	if (doc.type == 'participant') {
		emit([doc.event, doc.type, doc.date], {_id: doc.user});
	}
}
