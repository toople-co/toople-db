function(doc) {
	if (doc.type == 'invitation') {
		emit([doc.circle], {_id: doc.event});
	}
	if (doc.type == 'member') {
		emit([doc.circle, doc.date, doc._id], {_id: doc.user});
	}
}
